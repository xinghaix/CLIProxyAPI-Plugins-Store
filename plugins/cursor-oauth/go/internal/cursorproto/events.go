package cursorproto

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"
)

type EventKind string

const (
	EventIgnored    EventKind = "ignored"
	EventText       EventKind = "text"
	EventThinking   EventKind = "thinking"
	EventTokens     EventKind = "tokens"
	EventToolCall   EventKind = "tool_call"
	EventImage      EventKind = "image"
	EventCheckpoint EventKind = "checkpoint"
	EventDone       EventKind = "done"
)

type ServerEvent struct {
	Kind       EventKind
	Type       string
	Text       string
	Tokens     int
	ID         string
	Name       string
	Arguments  string
	MIMEType   string
	ImageData  []byte
	Path       string
	Checkpoint []byte
}

func DecodeServerEvent(raw []byte) (ServerEvent, error) {
	server, err := newMessage("AgentServerMessage")
	if err != nil {
		return ServerEvent{}, err
	}
	if err := proto.Unmarshal(raw, server); err != nil {
		return ServerEvent{}, fmt.Errorf("decode Cursor server message: %w", err)
	}
	messageOneof := server.Descriptor().Oneofs().ByName("message")
	active := server.WhichOneof(messageOneof)
	if active == nil {
		return ServerEvent{Kind: EventIgnored, Type: "empty"}, nil
	}
	if active.Name() == "conversation_checkpoint_update" {
		return decodeCheckpoint(server.Get(active).Message())
	}
	if active.Name() == "exec_server_message" {
		execField, err := requireField(server, "exec_server_message")
		if err != nil {
			return ServerEvent{}, err
		}
		return decodeExecServerMessage(server.Get(execField).Message())
	}
	if active.Name() == "interaction_query" {
		query := server.Get(active).Message()
		selected := query.WhichOneof(query.Descriptor().Oneofs().ByName("query"))
		if selected == nil {
			return ServerEvent{Kind: EventIgnored, Type: "interaction_query"}, nil
		}
		return ServerEvent{Kind: EventIgnored, Type: "interaction_query." + string(selected.Name())}, nil
	}
	if active.Name() != "interaction_update" {
		return ServerEvent{Kind: EventIgnored, Type: string(active.Name())}, nil
	}
	interactionField, err := requireField(server, "interaction_update")
	if err != nil {
		return ServerEvent{}, err
	}
	if !server.Has(interactionField) {
		return ServerEvent{Kind: EventIgnored, Type: "interaction_update"}, nil
	}
	return decodeInteraction(server.Get(interactionField).Message())
}

func decodeExecServerMessage(execMessage protoreflect.Message) (ServerEvent, error) {
	active := execMessage.WhichOneof(execMessage.Descriptor().Oneofs().ByName("message"))
	if active == nil {
		return ServerEvent{Kind: EventIgnored, Type: "exec_server_message"}, nil
	}
	if active.Name() != "mcp_args" {
		return ServerEvent{Kind: EventIgnored, Type: "exec_server_message." + string(active.Name())}, nil
	}
	argsField, err := requireField(execMessage, "mcp_args")
	if err != nil {
		return ServerEvent{}, err
	}
	args := execMessage.Get(argsField).Message()
	providerField, err := requireField(args, "provider_identifier")
	if err != nil {
		return ServerEvent{}, err
	}
	if args.Get(providerField).String() != toolProvider {
		return ServerEvent{Kind: EventIgnored, Type: "exec_server_message.mcp_args"}, nil
	}
	name := args.Get(field(args, "tool_name")).String()
	if name == "" {
		name = args.Get(field(args, "name")).String()
	}
	callID := args.Get(field(args, "tool_call_id")).String()
	if callID == "" {
		callID = fmt.Sprintf("exec_%d", execMessage.Get(field(execMessage, "id")).Uint())
	}
	arguments, err := decodeToolArguments(args)
	if err != nil {
		return ServerEvent{}, err
	}
	if name == "" {
		return ServerEvent{}, fmt.Errorf("Cursor MCP exec requires a tool name")
	}
	return ServerEvent{
		Kind: EventToolCall, Type: "exec_server_message.mcp_args", ID: callID, Name: name, Arguments: arguments,
	}, nil
}

func decodeInteraction(interaction protoreflect.Message) (ServerEvent, error) {
	active := interaction.WhichOneof(interaction.Descriptor().Oneofs().ByName("message"))
	if active == nil {
		return ServerEvent{Kind: EventIgnored, Type: "interaction_update"}, nil
	}
	switch active.Name() {
	case "text_delta", "thinking_delta", "token_delta", "tool_call_completed", "turn_ended":
		return eventFromField(active.Name(), interaction.Get(active).Message())
	default:
		return ServerEvent{Kind: EventIgnored, Type: "interaction_update." + string(active.Name())}, nil
	}
}

func eventFromField(name protoreflect.Name, message protoreflect.Message) (ServerEvent, error) {
	switch name {
	case "text_delta":
		return stringEvent(EventText, message)
	case "thinking_delta":
		return stringEvent(EventThinking, message)
	case "token_delta":
		descriptor, err := requireField(message, "tokens")
		if err != nil {
			return ServerEvent{}, err
		}
		return ServerEvent{Kind: EventTokens, Tokens: int(message.Get(descriptor).Int())}, nil
	case "turn_ended":
		return ServerEvent{Kind: EventDone, Type: string(name)}, nil
	case "tool_call_completed":
		return completedToolCallEvent(message)
	default:
		return ServerEvent{Kind: EventIgnored, Type: string(name)}, nil
	}
}

func completedToolCallEvent(completed protoreflect.Message) (ServerEvent, error) {
	callIDField, err := requireField(completed, "call_id")
	if err != nil {
		return ServerEvent{}, err
	}
	toolCallField, err := requireField(completed, "tool_call")
	if err != nil {
		return ServerEvent{}, err
	}
	if !completed.Has(toolCallField) {
		return ServerEvent{Kind: EventIgnored, Type: "tool_call_completed"}, nil
	}
	toolCall := completed.Get(toolCallField).Message()
	active := toolCall.WhichOneof(toolCall.Descriptor().Oneofs().ByName("tool"))
	if active == nil {
		return ServerEvent{Kind: EventIgnored, Type: "tool_call_completed"}, nil
	}
	if active.Name() == "generate_image_tool_call" {
		return completedImageToolCallEvent(completed, toolCall.Get(active).Message())
	}
	if active.Name() != "mcp_tool_call" {
		return ServerEvent{Kind: EventIgnored, Type: "tool_call_completed." + string(active.Name())}, nil
	}
	mcpCall := toolCall.Get(active).Message()
	argsField, err := requireField(mcpCall, "args")
	if err != nil {
		return ServerEvent{}, err
	}
	if !mcpCall.Has(argsField) {
		return ServerEvent{Kind: EventIgnored, Type: "tool_call_completed"}, nil
	}
	args := mcpCall.Get(argsField).Message()
	providerField, err := requireField(args, "provider_identifier")
	if err != nil {
		return ServerEvent{}, err
	}
	if args.Get(providerField).String() != toolProvider {
		return ServerEvent{Kind: EventIgnored, Type: "tool_call_completed"}, nil
	}
	name := args.Get(field(args, "tool_name")).String()
	if name == "" {
		name = args.Get(field(args, "name")).String()
	}
	callID := completed.Get(callIDField).String()
	if callID == "" {
		callID = args.Get(field(args, "tool_call_id")).String()
	}
	arguments, err := decodeToolArguments(args)
	if err != nil {
		return ServerEvent{}, err
	}
	if callID == "" || name == "" {
		return ServerEvent{}, fmt.Errorf("Cursor completed tool call requires id and name")
	}
	return ServerEvent{Kind: EventToolCall, Type: "tool_call_completed", ID: callID, Name: name, Arguments: arguments}, nil
}

func completedImageToolCallEvent(completed, imageCall protoreflect.Message) (ServerEvent, error) {
	callIDField, err := requireField(completed, "call_id")
	if err != nil {
		return ServerEvent{}, err
	}
	callID := completed.Get(callIDField).String()
	if callID == "" {
		return ServerEvent{}, fmt.Errorf("Cursor completed image generation requires a call id")
	}
	resultField, err := requireField(imageCall, "result")
	if err != nil {
		return ServerEvent{}, err
	}
	if !imageCall.Has(resultField) {
		return ServerEvent{}, fmt.Errorf("Cursor image generation %q completed without a result", callID)
	}
	result := imageCall.Get(resultField).Message()
	active := result.WhichOneof(result.Descriptor().Oneofs().ByName("result"))
	if active == nil {
		return ServerEvent{}, fmt.Errorf("Cursor image generation %q returned an empty result", callID)
	}
	if active.Name() == "error" {
		failure := result.Get(active).Message()
		errorField, fieldErr := requireField(failure, "error")
		if fieldErr != nil {
			return ServerEvent{}, fieldErr
		}
		message := strings.TrimSpace(failure.Get(errorField).String())
		if message == "" {
			message = "unknown upstream error"
		}
		return ServerEvent{}, fmt.Errorf("Cursor image generation failed: %s", message)
	}
	if active.Name() != "success" {
		return ServerEvent{}, fmt.Errorf("Cursor image generation %q returned unsupported result %q", callID, active.Name())
	}
	success := result.Get(active).Message()
	dataField, err := requireField(success, "image_data")
	if err != nil {
		return ServerEvent{}, err
	}
	encoded := strings.TrimSpace(success.Get(dataField).String())
	if encoded == "" {
		return ServerEvent{}, fmt.Errorf("Cursor image generation %q returned no image data", callID)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ServerEvent{}, fmt.Errorf("decode Cursor generated image %q: %w", callID, err)
	}
	pathField, err := requireField(success, "file_path")
	if err != nil {
		return ServerEvent{}, err
	}
	path := success.Get(pathField).String()
	mimeType := generatedImageMIME(data, path)
	if !strings.HasPrefix(mimeType, "image/") {
		return ServerEvent{}, fmt.Errorf("Cursor image generation %q returned non-image data (%s)", callID, mimeType)
	}
	return ServerEvent{
		Kind: EventImage, Type: "tool_call_completed.generate_image_tool_call", ID: callID,
		MIMEType: mimeType, ImageData: data, Path: path,
	}, nil
}

func generatedImageMIME(data []byte, path string) string {
	detected := http.DetectContentType(data)
	if detected != "application/octet-stream" {
		if mediaType, _, err := mime.ParseMediaType(detected); err == nil {
			return mediaType
		}
		return detected
	}
	if extensionType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); extensionType != "" {
		if mediaType, _, err := mime.ParseMediaType(extensionType); err == nil {
			return mediaType
		}
		return extensionType
	}
	return detected
}

func decodeToolArguments(args protoreflect.Message) (string, error) {
	descriptor, err := requireField(args, "args")
	if err != nil {
		return "", err
	}
	decoded := make(map[string]any)
	args.Get(descriptor).Map().Range(func(key protoreflect.MapKey, value protoreflect.Value) bool {
		decoded[key.String()] = decodeToolArgument(value.Bytes())
		return true
	})
	raw, err := json.Marshal(decoded)
	if err != nil {
		return "", fmt.Errorf("encode Cursor tool arguments: %w", err)
	}
	return string(raw), nil
}

func decodeToolArgument(raw []byte) any {
	var value structpb.Value
	if err := proto.Unmarshal(raw, &value); err == nil && value.Kind != nil && len(value.ProtoReflect().GetUnknown()) == 0 {
		decoded := value.AsInterface()
		if text, ok := decoded.(string); ok {
			var parsed any
			if json.Unmarshal([]byte(text), &parsed) == nil {
				return parsed
			}
		}
		return decoded
	}
	var decoded any
	if json.Unmarshal(raw, &decoded) == nil {
		return decoded
	}
	return string(raw)
}

func stringEvent(kind EventKind, message protoreflect.Message) (ServerEvent, error) {
	descriptor, err := requireField(message, "text")
	if err != nil {
		return ServerEvent{}, err
	}
	return ServerEvent{Kind: kind, Type: string(message.Descriptor().Name()), Text: message.Get(descriptor).String()}, nil
}

func DecodeModels(raw []byte) ([]string, error) {
	response, err := newMessage("GetUsableModelsResponse")
	if err != nil {
		return nil, err
	}
	if err := proto.Unmarshal(raw, response); err != nil {
		return nil, fmt.Errorf("decode Cursor models response: %w", err)
	}
	modelsField, err := requireField(response, "models")
	if err != nil {
		return nil, err
	}
	models := response.Get(modelsField).List()
	ids := make([]string, 0, models.Len())
	for index := range models.Len() {
		model := models.Get(index).Message()
		idField, err := requireField(model, "model_id")
		if err != nil {
			return nil, err
		}
		if id := model.Get(idField).String(); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
