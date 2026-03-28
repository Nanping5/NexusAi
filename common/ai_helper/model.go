package aihelper

import (
	"NexusAi/common/rag"
	"NexusAi/model"
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type StreamCallback func(msg string)

type AIModel interface {
	GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
	StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error)
	GetModelType() string
	GetModelName() string
}

// streamResponseFromModel 通用的流式响应处理逻辑
func streamResponseFromModel(ctx context.Context, chatModel einomodel.ToolCallingChatModel, messages []*schema.Message, cb StreamCallback, modelType string) (string, error) {
	stream, err := chatModel.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("%s stream failed: %v", modelType, err)
	}
	defer stream.Close()

	var fullResponse strings.Builder

	for {
		select {
		case <-ctx.Done():
			// 客户端断开连接或请求被取消
			return "", ctx.Err()
		default:
			msg, err := stream.Recv()

			if err == io.EOF {
				return fullResponse.String(), nil
			}
			if err != nil {
				return "", fmt.Errorf("%s stream recv failed: %v", modelType, err)
			}

			if len(msg.Content) > 0 {
				fullResponse.WriteString(msg.Content) // 累积完整响应
				cb(msg.Content)                       // 实时回调当前消息片段
			}
		}
	}
}

// OpenAICompatibleModel OpenAI 兼容模型实现（支持动态配置）
type OpenAICompatibleModel struct {
	llm       einomodel.ToolCallingChatModel
	config    *model.AIModelConfig
	userID    string
	sessionID string
}

// NewOpenAICompatibleModel 创建 OpenAI 兼容模型实例（动态配置）
func NewOpenAICompatibleModel(ctx context.Context, config *model.AIModelConfig, userID, sessionID string) (*OpenAICompatibleModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is required for model %s", config.Name)
	}

	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:   config.ModelName,
		APIKey:  config.APIKey,
		BaseURL: config.BaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("create openai compatible model failed: %v", err)
	}
	return &OpenAICompatibleModel{llm: llm, config: config, userID: userID, sessionID: sessionID}, nil
}

// GenerateResponse 生成完整响应（支持 RAG 检索）
func (o *OpenAICompatibleModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	// 尝试使用 RAG 增强上下文
	ragMessages, err := o.enhanceWithRAG(ctx, messages)
	if err != nil {
		log.Printf("RAG enhancement failed, using original messages: %v", err)
		ragMessages = messages
	}

	resp, err := o.llm.Generate(ctx, ragMessages)
	if err != nil {
		return nil, fmt.Errorf("model generate failed: %v", err)
	}
	return resp, nil
}

// StreamResponse 实现流式响应（支持 RAG 检索）
func (o *OpenAICompatibleModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	// 尝试使用 RAG 增强上下文
	ragMessages, err := o.enhanceWithRAG(ctx, messages)
	if err != nil {
		log.Printf("RAG enhancement failed, using original messages: %v", err)
		ragMessages = messages
	}

	return streamResponseFromModel(ctx, o.llm, ragMessages, cb, o.config.Name)
}

// enhanceWithRAG 使用 RAG 检索增强消息上下文
func (o *OpenAICompatibleModel) enhanceWithRAG(ctx context.Context, messages []*schema.Message) ([]*schema.Message, error) {
	// 1. 创建 RAG 查询器
	ragQuery, err := rag.NewRAGQuery(ctx, o.sessionID)
	if err != nil {
		log.Printf("[RAG DEBUG] Failed to create RAG query: %v", err)
		return nil, fmt.Errorf("failed to create RAG query: %v", err)
	}

	// 2. 获取用户最后一条消息作为查询（倒序遍历找到最后一个 User 角色的消息）
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	var query string
	foundUserMsg := false
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == schema.User {
			query = messages[i].Content
			foundUserMsg = true
			log.Printf("[RAG DEBUG] Found user message at index %d: %s", i, query)
			break
		}
	}

	if !foundUserMsg {
		return nil, fmt.Errorf("no user message found in conversation")
	}

	// 3. 检索相关文档
	docs, err := ragQuery.RetrieveDocuments(ctx, query)
	if err != nil {
		log.Printf("[RAG DEBUG] Failed to retrieve documents: %v", err)
		return nil, fmt.Errorf("failed to retrieve documents: %v", err)
	}

	log.Printf("[RAG DEBUG] Retrieved %d documents for session %s", len(docs), o.sessionID)
	for i, doc := range docs {
		log.Printf("[RAG DEBUG] Doc %d: %s...", i, doc.Content[:min(100, len(doc.Content))])
	}

	// 4. 构建包含检索结果的提示词
	ragPrompt := rag.BuildRagPrompt(query, docs)
	log.Printf("[RAG DEBUG] RAG prompt length: %d", len(ragPrompt))

	// 5. 替换最后一条用户消息为 RAG 提示词（保持其他消息不变）
	ragMessages := make([]*schema.Message, len(messages))
	copy(ragMessages, messages)

	for i := len(ragMessages) - 1; i >= 0; i-- {
		if ragMessages[i].Role == schema.User {
			ragMessages[i] = &schema.Message{
				Role:    schema.User,
				Content: ragPrompt,
			}
			break
		}
	}

	return ragMessages, nil
}

func (o *OpenAICompatibleModel) GetModelType() string {
	return o.config.ModelType
}

// GetModelName 获取模型显示名称
func (o *OpenAICompatibleModel) GetModelName() string {
	return o.config.Name
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
