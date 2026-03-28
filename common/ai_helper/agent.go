package aihelper

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"NexusAi/common/mcpmanager"
	"NexusAi/common/rag"
	"NexusAi/model"
	mylogger "NexusAi/pkg/logger"

	"github.com/cloudwego/eino-ext/components/model/openai"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// AgentMode Agent 模式类型
type AgentMode string

const (
	AgentModeNone  AgentMode = "none"
	AgentModeReAct AgentMode = "react"
	AgentModePlan  AgentMode = "plan"
)

// Agent 配置常量
const (
	AgentSystemPrompt = `你是一个智能助手，可以使用工具来帮助用户。

【重要规则】
1. 当用户请求涉及以下场景时，你必须立即调用相应工具，不要只是说"我来帮你..."而不行动：
   - 搜索、查询、最新消息、新闻、资讯 → 必须调用 search 工具
   - 朗读、播放、语音、念给我听 → 必须调用 text_to_speech 工具
   - 翻译 → 必须调用 translate 工具
   - 天气 → 必须调用 weather 工具

2. 禁止行为：
   - 禁止在没有调用工具的情况下说"我来为您搜索..."
   - 禁止在没有调用工具的情况下说"正在为您查询..."
   - 如果需要使用工具，直接调用，不要先用文字描述

3. 工具调用后，基于工具返回的结果给出自然、有帮助的回复。`
)

// AgentConfig Agent 配置
type AgentConfig struct {
	Mode          AgentMode // Agent 模式
	MaxIterations int       // 最大迭代次数（Agent 执行步骤限制）
}

// AgentService Agent 服务，封装 ReAct Agent 逻辑
type AgentService struct {
	agent     *react.Agent
	model     einomodel.ToolCallingChatModel
	modelName string
	userID    string
	sessionID string
	config    AgentConfig
}

// NewAgentServiceByConfigID 根据配置 ID 创建 Agent 服务实例
func NewAgentServiceByConfigID(ctx context.Context, configID, userID, sessionID string, agentConfig AgentConfig) (*AgentService, error) {
	// 获取模型配置
	config, err := GetConfigByConfigID(ctx, configID)
	if err != nil {
		return nil, fmt.Errorf("failed to get model config: %w", err)
	}
	if config == nil {
		return nil, fmt.Errorf("model config not found: %s", configID)
	}
	if !config.IsEnabled {
		return nil, fmt.Errorf("model %s is disabled", config.Name)
	}

	return NewAgentServiceFromConfig(ctx, config, userID, sessionID, agentConfig)
}

// NewAgentServiceFromConfig 根据配置创建 Agent 服务实例
func NewAgentServiceFromConfig(ctx context.Context, config *model.AIModelConfig, userID, sessionID string, agentConfig AgentConfig) (*AgentService, error) {
	// 创建聊天模型
	chatModel, err := createChatModelFromConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat model: %w", err)
	}

	// 设置默认配置
	if agentConfig.MaxIterations <= 0 {
		agentConfig.MaxIterations = 10
	}
	if agentConfig.Mode == "" {
		agentConfig.Mode = AgentModeReAct
	}

	return &AgentService{
		model:     chatModel,
		modelName: config.Name,
		userID:    userID,
		sessionID: sessionID,
		config:    agentConfig,
	}, nil
}

// createChatModelFromConfig 根据配置创建聊天模型
func createChatModelFromConfig(ctx context.Context, config *model.AIModelConfig) (einomodel.ToolCallingChatModel, error) {
	if config.ModelType != "openai_compatible" {
		return nil, fmt.Errorf("unsupported model type: %s, only openai_compatible is supported", config.ModelType)
	}

	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is required for model %s", config.Name)
	}
	return openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:   config.ModelName,
		APIKey:  config.APIKey,
		BaseURL: config.BaseURL,
	})
}

// InitializeAgent 初始化 Agent（加载 MCP 工具）
func (a *AgentService) InitializeAgent(ctx context.Context) error {
	// 获取 MCP 管理器
	mcpManager := mcpmanager.GetGlobalMCPManager()

	// 获取所有 MCP 工具
	tools, err := mcpManager.GetAllTools(ctx)
	if err != nil {
		mylogger.Logger.Warn("failed to get MCP tools, agent will run without tools",
			zap.Error(err),
		)
		tools = nil
	}

	// 创建 ReAct Agent
	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: a.model,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools,
		},
		MaxStep: a.config.MaxIterations,
		// 使用 MessageModifier 添加系统提示词，强制 Agent 调用工具
		MessageModifier: func(ctx context.Context, input []*schema.Message) []*schema.Message {
			// 在消息列表开头插入系统提示词
			systemMsg := &schema.Message{
				Role:    schema.System,
				Content: AgentSystemPrompt,
			}
			result := make([]*schema.Message, 0, len(input)+1)
			result = append(result, systemMsg)
			result = append(result, input...)
			return result
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create react agent: %w", err)
	}

	a.agent = agent
	mylogger.Logger.Info("Agent initialized successfully",
		zap.String("modelName", a.modelName),
		zap.String("mode", string(a.config.Mode)),
		zap.Int("toolCount", len(tools)),
	)

	return nil
}

// GenerateResponse 使用 Agent 生成响应
func (a *AgentService) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	if a.agent == nil {
		if err := a.InitializeAgent(ctx); err != nil {
			return nil, err
		}
	}

	// 使用 RAG 增强消息上下文
	ragMessages, err := a.enhanceWithRAG(ctx, messages)
	if err != nil {
		log.Printf("[RAG DEBUG] Agent RAG enhancement failed: %v", err)
		ragMessages = messages
	}

	resp, err := a.agent.Generate(ctx, ragMessages)
	if err != nil {
		return nil, fmt.Errorf("agent generate failed: %w", err)
	}

	return resp, nil
}

// StreamResponse 使用 Agent 生成流式响应
func (a *AgentService) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	if a.agent == nil {
		if err := a.InitializeAgent(ctx); err != nil {
			return "", err
		}
	}

	// 使用 RAG 增强消息上下文
	ragMessages, err := a.enhanceWithRAG(ctx, messages)
	if err != nil {
		log.Printf("[RAG DEBUG] Agent RAG enhancement failed: %v", err)
		ragMessages = messages
	}

	stream, err := a.agent.Stream(ctx, ragMessages)
	if err != nil {
		return "", fmt.Errorf("agent stream failed: %w", err)
	}
	defer stream.Close()

	var fullResponse strings.Builder

	for {
		select {
		case <-ctx.Done():
			return fullResponse.String(), ctx.Err()
		default:
			msg, err := stream.Recv()

			if err == io.EOF {
				return fullResponse.String(), nil
			}
			if err != nil {
				mylogger.Logger.Error("agent stream recv error", zap.Error(err))
				// 如果已经有部分响应，返回它而不是错误
				if fullResponse.Len() > 0 {
					return fullResponse.String(), nil
				}
				return "", fmt.Errorf("agent stream recv failed: %w", err)
			}

			// 处理消息内容
			if len(msg.Content) > 0 {
				fullResponse.WriteString(msg.Content)
				cb(msg.Content)
			}
		}
	}
}

// GetModelType 获取模型类型
func (a *AgentService) GetModelType() string {
	return a.modelName
}

// GetModelName 获取模型显示名称
func (a *AgentService) GetModelName() string {
	return a.modelName
}

// enhanceWithRAG 使用 RAG 检索增强消息上下文
func (a *AgentService) enhanceWithRAG(ctx context.Context, messages []*schema.Message) ([]*schema.Message, error) {
	// 创建 RAG 查询器
	ragQuery, err := rag.NewRAGQuery(ctx, a.sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to create RAG query: %v", err)
	}

	// 获取用户最后一条消息作为查询（倒序遍历找到最后一个 User 角色的消息）
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	var query string
	foundUserMsg := false
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == schema.User {
			query = messages[i].Content
			foundUserMsg = true
			log.Printf("[RAG DEBUG] Agent found user message at index %d: %s", i, query)
			break
		}
	}

	if !foundUserMsg {
		return nil, fmt.Errorf("no user message found in conversation")
	}

	// 3. 检索相关文档
	docs, err := ragQuery.RetrieveDocuments(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve documents: %v", err)
	}

	log.Printf("[RAG DEBUG] Agent retrieved %d documents for session %s", len(docs), a.sessionID)

	// 4. 构建包含检索结果的提示词
	ragPrompt := rag.BuildRagPrompt(query, docs)

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
