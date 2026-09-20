package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cursor-oauth/go/internal/cursorauth"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cursor-oauth/go/internal/cursorproto"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cursor-oauth/go/internal/openai"
)

func (handler *Handler) execute(ctx context.Context, raw []byte) (any, error) {
	request, chat, credentials, err := decodeExecution(raw)
	if err != nil {
		return nil, err
	}
	turn := openai.NewTurn("cursor/" + chat.Model)
	defer func() {
		handler.usage.recordTokens(request.AuthID, turn.EstimatedUsage(usageText(chat)))
	}()
	if handler.cursor == nil {
		return nil, errors.New("Cursor client is unavailable")
	}
	_, err = handler.runCheckpointed(ctx, request, chat, credentials, func(event cursorproto.ServerEvent) error {
		switch event.Kind {
		case cursorproto.EventText:
			turn.AddText(event.Text)
		case cursorproto.EventThinking:
			turn.AddReasoning(event.Text)
		case cursorproto.EventImage:
			turn.AddImage(event.MIMEType, event.ImageData)
		case cursorproto.EventToolCall:
			turn.AddToolCall(event.ID, event.Name, event.Arguments)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	payload, err := turn.Completion(usageText(chat))
	if err != nil {
		return nil, err
	}
	return executorResponse{Payload: payload, Headers: http.Header{"content-type": []string{"application/json"}}}, nil
}

func (handler *Handler) executeStream(ctx context.Context, raw []byte) (any, error) {
	request, chat, credentials, err := decodeExecution(raw)
	if err != nil {
		return nil, err
	}
	if handler.cursor == nil || handler.emitter == nil || request.StreamID == "" {
		turn := openai.NewTurn("cursor/" + chat.Model)
		handler.usage.recordTokens(request.AuthID, turn.EstimatedUsage(usageText(chat)))
		return nil, errors.New("Cursor stream bridge is unavailable")
	}
	go handler.runStream(ctx, request, chat, credentials)
	return struct {
		Headers http.Header `json:"headers"`
	}{Headers: http.Header{"content-type": []string{"text/event-stream"}}}, nil
}

func (handler *Handler) runStream(parent context.Context, request executorRequest, chat openai.ChatRequest, credentials cursorauth.Credentials) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()
	turn := openai.NewTurn("cursor/" + chat.Model)
	done := false
	toolCallSeen := false
	var runErr error
	defer func() {
		handler.usage.recordTokens(request.AuthID, turn.EstimatedUsage(usageText(chat)))
	}()
	_, runErr = handler.runCheckpointed(ctx, request, chat, credentials, func(event cursorproto.ServerEvent) error {
		switch event.Kind {
		case cursorproto.EventText:
			chunk, err := turn.StreamChunk(event.Text)
			if err != nil {
				return err
			}
			return handler.emitter.Emit(ctx, request.StreamID, chunk)
		case cursorproto.EventImage:
			chunk, err := turn.StreamImage(event.MIMEType, event.ImageData)
			if err != nil {
				return err
			}
			return handler.emitter.Emit(ctx, request.StreamID, chunk)
		case cursorproto.EventDone:
			done = true
			chunk, err := turn.FinalChunk(usageText(chat))
			if err != nil {
				return err
			}
			if err := handler.emitter.Emit(ctx, request.StreamID, chunk); err != nil {
				return err
			}
			return handler.emitter.Emit(ctx, request.StreamID, []byte("[DONE]"))
		case cursorproto.EventToolCall:
			chunk, err := turn.StreamToolCall(event.ID, event.Name, event.Arguments)
			if err != nil {
				return err
			}
			if err := handler.emitter.Emit(ctx, request.StreamID, chunk); err != nil {
				return err
			}
			toolCallSeen = true
			return nil
		case cursorproto.EventThinking:
			chunk, err := turn.StreamReasoning(event.Text)
			if err != nil {
				return err
			}
			return handler.emitter.Emit(ctx, request.StreamID, chunk)
		case cursorproto.EventIgnored, cursorproto.EventTokens:
			return nil
		default:
			return nil
		}
	})
	if runErr == nil && toolCallSeen && !done {
		chunk, err := turn.FinalChunk(usageText(chat))
		if err != nil {
			runErr = err
		} else if err = handler.emitter.Emit(ctx, request.StreamID, chunk); err != nil {
			runErr = err
		} else if err = handler.emitter.Emit(ctx, request.StreamID, []byte("[DONE]")); err != nil {
			runErr = err
		} else {
			done = true
		}
	}
	if runErr == nil && !done {
		runErr = errors.New("Cursor stream completed without turn end")
	}
	if closeErr := handler.emitter.Close(request.StreamID, runErr); closeErr != nil {
		return
	}
}

func cursorTools(tools []openai.Tool) []cursorproto.ToolDefinition {
	result := make([]cursorproto.ToolDefinition, len(tools))
	for index, tool := range tools {
		result[index] = cursorproto.ToolDefinition{Name: tool.Name, Description: tool.Description, Parameters: tool.Parameters}
	}
	return result
}

func cursorImages(images []openai.Image) []cursorproto.ImageAttachment {
	result := make([]cursorproto.ImageAttachment, len(images))
	for index, image := range images {
		result[index] = cursorproto.ImageAttachment{Name: image.Name, MIMEType: image.MIMEType, Data: image.Data}
	}
	return result
}

func cursorAttachments(attachments []openai.Attachment) []cursorproto.FileAttachment {
	result := make([]cursorproto.FileAttachment, len(attachments))
	for index, attachment := range attachments {
		result[index] = cursorproto.FileAttachment{Name: attachment.Name, Content: attachment.Content}
	}
	return result
}

func usageText(chat openai.ChatRequest) string {
	text := chat.System + chat.Prompt
	for _, attachment := range chat.Attachments {
		text += attachment.Content
	}
	for _, tool := range chat.Tools {
		text += tool.Name + tool.Description + string(tool.Parameters)
	}
	return text
}

func decodeExecution(raw []byte) (executorRequest, openai.ChatRequest, cursorauth.Credentials, error) {
	var request executorRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return executorRequest{}, openai.ChatRequest{}, cursorauth.Credentials{}, fmt.Errorf("decode executor request: %w", err)
	}
	payload := request.Payload
	if len(payload) == 0 {
		payload = request.OriginalRequest
	}
	chat, err := openai.ParseChatRequest(payload)
	if err != nil {
		return executorRequest{}, openai.ChatRequest{}, cursorauth.Credentials{}, err
	}
	credentials, err := cursorauth.ParseCredentials(request.StorageJSON)
	if err != nil {
		return executorRequest{}, openai.ChatRequest{}, cursorauth.Credentials{}, err
	}
	if _, disabled := normalizedModelSet(credentials.DisabledModels)[chat.Model]; disabled {
		return executorRequest{}, openai.ChatRequest{}, cursorauth.Credentials{}, &openai.InvalidRequestError{
			Message: fmt.Sprintf("Cursor model %q is disabled by plugin configuration", chat.Model),
		}
	}
	if err := chat.ValidateToolProgress(credentials.ToolLoopGuardTools); err != nil {
		return executorRequest{}, openai.ChatRequest{}, cursorauth.Credentials{}, err
	}
	return request, chat, credentials, nil
}

func countTokens(raw []byte) (any, error) {
	var request executorRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, fmt.Errorf("decode token request: %w", err)
	}
	payload := request.Payload
	if len(payload) == 0 {
		payload = request.OriginalRequest
	}
	chat, err := openai.ParseChatRequest(payload)
	if err != nil {
		return nil, err
	}
	tokens := max(1, utf8.RuneCountInString(usageText(chat))/4)
	encoded, err := json.Marshal(map[string]int{"total_tokens": tokens})
	if err != nil {
		return nil, fmt.Errorf("encode token count: %w", err)
	}
	return executorResponse{Payload: encoded, Headers: http.Header{"content-type": []string{"application/json"}}}, nil
}
