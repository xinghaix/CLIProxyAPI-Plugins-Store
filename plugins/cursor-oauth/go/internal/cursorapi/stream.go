package cursorapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cursor-oauth/go/internal/cursorproto"
)

type bodyRead struct {
	data []byte
	err  error
}

const connectEndStreamFlag byte = 0x02

type streamState struct {
	result      RunResult
	started     time.Time
	terminal    bool
	contentSeen bool
	doneSent    bool
	drain       *time.Timer
	eventCounts map[string]int
}

func readRunStream(
	ctx context.Context,
	body io.ReadCloser,
	result RunResult,
	emit func(cursorproto.ServerEvent) error,
	blobs *cursorproto.BlobStore,
	imageWrites *cursorproto.ImageWriteExecutor,
	environment cursorproto.RequestEnvironment,
	outbound chan<- []byte,
	watchdogs *runWatchdogs,
	drainDelay time.Duration,
) (RunResult, error) {
	state := streamState{result: result, started: time.Now()}
	decoder := cursorproto.NewFrameDecoder(32 << 20)
	reads := startBodyReader(body)
	defer stopBodyReader(body, reads)
	for {
		read, drainExpired, err := nextBodyRead(ctx, reads, state.drain)
		if err != nil {
			return state.result, state.annotateProgressTimeout(runCause(ctx, err))
		}
		if drainExpired {
			return state.result, nil
		}
		if len(read.data) > 0 {
			watchdogs.sawData()
			frames, err := decoder.Push(read.data)
			if err != nil {
				return state.result, err
			}
			for _, frame := range frames {
				watchdogs.sawFrame()
				finished, err := state.handleFrame(ctx, frame, emit, blobs, imageWrites, environment, outbound, watchdogs, drainDelay)
				if err != nil || finished {
					return state.result, err
				}
			}
		}
		if errors.Is(read.err, io.EOF) {
			if state.terminal {
				return state.result, nil
			}
			return state.result, errors.New("Cursor stream closed before turn end")
		}
		if read.err != nil {
			return state.result, runCause(ctx, fmt.Errorf("read Cursor stream: %w", read.err))
		}
	}
}

func (state *streamState) handleFrame(
	ctx context.Context,
	frame cursorproto.Frame,
	emit func(cursorproto.ServerEvent) error,
	blobs *cursorproto.BlobStore,
	imageWrites *cursorproto.ImageWriteExecutor,
	environment cursorproto.RequestEnvironment,
	outbound chan<- []byte,
	watchdogs *runWatchdogs,
	drainDelay time.Duration,
) (bool, error) {
	if frame.Flags == connectEndStreamFlag {
		if err := connectEndStreamError(frame.Payload); err != nil {
			return false, err
		}
		if state.terminal {
			return true, nil
		}
		if !state.contentSeen {
			return false, ErrEmptyCompletion
		}
		if err := emit(cursorproto.ServerEvent{Kind: cursorproto.EventDone, Type: "connect_end_stream"}); err != nil {
			return false, fmt.Errorf("emit Cursor event: %w", err)
		}
		state.terminal = true
		state.doneSent = true
		watchdogs.stop()
		return true, nil
	}
	if frame.Flags != 0 {
		return false, fmt.Errorf("Cursor stream ended with Connect flags %d", frame.Flags)
	}
	reply, handled, err := blobs.HandleServerMessage(frame.Payload)
	if err != nil {
		return false, err
	}
	if handled {
		select {
		case outbound <- reply:
		case <-ctx.Done():
			return false, context.Cause(ctx)
		}
	}
	reply, handled, err = cursorproto.ReplyRequestContext(frame.Payload, environment)
	if err != nil {
		return false, err
	}
	if handled {
		select {
		case outbound <- reply:
			watchdogs.sawProgress()
		case <-ctx.Done():
			return false, context.Cause(ctx)
		}
	}
	reply, handled, err = imageWrites.HandleServerMessage(frame.Payload)
	if err != nil {
		return false, err
	}
	if handled {
		select {
		case outbound <- reply:
			watchdogs.sawProgress()
		case <-ctx.Done():
			return false, context.Cause(ctx)
		}
	}
	reply, handled, err = cursorproto.ApproveGenerateImageRequest(frame.Payload)
	if err != nil {
		return false, err
	}
	if handled {
		select {
		case outbound <- reply:
			watchdogs.sawProgress()
		case <-ctx.Done():
			return false, context.Cause(ctx)
		}
	}
	reply, handled, err = cursorproto.ReplyNativeReadOnlyExec(frame.Payload)
	if err != nil {
		return false, err
	}
	if handled {
		select {
		case outbound <- reply:
			watchdogs.sawProgress()
		case <-ctx.Done():
			return false, context.Cause(ctx)
		}
	}
	shellReplies, handled, err := cursorproto.ReplyNativeShellExec(frame.Payload)
	if err != nil {
		return false, err
	}
	if handled {
		for _, shellReply := range shellReplies {
			select {
			case outbound <- shellReply:
				watchdogs.sawProgress()
			case <-ctx.Done():
				return false, context.Cause(ctx)
			}
		}
	}
	event, err := cursorproto.DecodeServerEvent(frame.Payload)
	if err != nil {
		return false, err
	}
	state.recordEvent(event)
	if event.Kind == cursorproto.EventCheckpoint {
		state.result.Checkpoint = append(state.result.Checkpoint[:0], event.Checkpoint...)
		return state.terminal, nil
	}
	if state.terminal {
		if event.Kind == cursorproto.EventDone && !state.doneSent {
			if err := emit(event); err != nil {
				return false, fmt.Errorf("emit Cursor event: %w", err)
			}
			state.doneSent = true
			state.result.OutputExposed = true
		}
		return false, nil
	}
	if eventMakesProgress(event.Kind) {
		watchdogs.sawProgress()
		if state.result.TTFT == 0 && event.Kind != cursorproto.EventDone {
			state.result.TTFT = time.Since(state.started)
		}
	}
	state.result.OutputExposed = state.result.OutputExposed || eventExposesOutput(event.Kind)
	state.result.ToolExposed = state.result.ToolExposed || event.Kind == cursorproto.EventToolCall
	state.contentSeen = state.contentSeen || event.Kind == cursorproto.EventText || event.Kind == cursorproto.EventImage
	if err := emit(event); err != nil {
		return false, fmt.Errorf("emit Cursor event: %w", err)
	}
	state.doneSent = state.doneSent || event.Kind == cursorproto.EventDone
	if event.Kind == cursorproto.EventDone || event.Kind == cursorproto.EventToolCall {
		state.terminal = true
		watchdogs.stop()
		state.drain = time.NewTimer(drainDelay)
	}
	return false, nil
}

func (state *streamState) recordEvent(event cursorproto.ServerEvent) {
	eventType := event.Type
	if eventType == "" {
		eventType = string(event.Kind)
	}
	if state.eventCounts == nil {
		state.eventCounts = make(map[string]int)
	}
	state.eventCounts[eventType]++
}

func (state *streamState) annotateProgressTimeout(err error) error {
	if !errors.Is(err, ErrProgressTimeout) || len(state.eventCounts) == 0 {
		return err
	}
	types := make([]string, 0, len(state.eventCounts))
	for eventType := range state.eventCounts {
		types = append(types, eventType)
	}
	sort.Strings(types)
	counts := make([]string, 0, len(types))
	for _, eventType := range types {
		counts = append(counts, fmt.Sprintf("%s=%d", eventType, state.eventCounts[eventType]))
	}
	return fmt.Errorf("%w (Cursor event counts: %s)", err, strings.Join(counts, ", "))
}

func eventMakesProgress(kind cursorproto.EventKind) bool {
	switch kind {
	case cursorproto.EventText, cursorproto.EventThinking, cursorproto.EventTokens, cursorproto.EventToolCall, cursorproto.EventImage, cursorproto.EventDone:
		return true
	case cursorproto.EventIgnored, cursorproto.EventCheckpoint:
		return false
	default:
		return false
	}
}

func eventExposesOutput(kind cursorproto.EventKind) bool {
	switch kind {
	case cursorproto.EventText, cursorproto.EventThinking, cursorproto.EventTokens, cursorproto.EventImage, cursorproto.EventDone:
		return true
	case cursorproto.EventIgnored, cursorproto.EventToolCall, cursorproto.EventCheckpoint:
		return false
	default:
		return false
	}
}

func startBodyReader(body io.Reader) <-chan bodyRead {
	result := make(chan bodyRead, 1)
	go func() {
		defer close(result)
		for {
			buffer := make([]byte, 32<<10)
			count, err := body.Read(buffer)
			result <- bodyRead{data: append([]byte(nil), buffer[:count]...), err: err}
			if err != nil {
				return
			}
		}
	}()
	return result
}

func stopBodyReader(body io.Closer, reads <-chan bodyRead) {
	_ = body.Close()
	for range reads {
	}
}

func nextBodyRead(ctx context.Context, reads <-chan bodyRead, drain *time.Timer) (bodyRead, bool, error) {
	var drainChannel <-chan time.Time
	if drain != nil {
		drainChannel = drain.C
	}
	select {
	case read := <-reads:
		return read, false, nil
	case <-drainChannel:
		return bodyRead{}, true, nil
	case <-ctx.Done():
		return bodyRead{}, false, context.Cause(ctx)
	}
}
