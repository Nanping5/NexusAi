package rag

import (
	"NexusAi/common/qdrant"
	"NexusAi/config"
	mylogger "NexusAi/pkg/logger"
	"NexusAi/pkg/utils"
	"context"
	"fmt"
	"os"
	"strings"

	embeddingArk "github.com/cloudwego/eino-ext/components/embedding/ark"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/schema"
	qdrantgo "github.com/qdrant/go-client/qdrant"
	"go.uber.org/zap"
)

// RAGIndexer RAG 索引器，负责文档向量化存储
type RAGIndexer struct {
	embedding  embedding.Embedder
	client     *qdrantgo.Client
	collection string
	vectorSize uint64
}

// RAGQuery RAG 查询器，负责向量检索
type RAGQuery struct {
	embedding  embedding.Embedder
	client     *qdrantgo.Client
	collection string
	sessionID  string
}

// getCollectionName 获取集合名称
func getCollectionName() string {
	collectionName := config.GetConfig().QdrantConfig.Collection
	if collectionName == "" {
		collectionName = "nexus_rag"
	}
	return collectionName
}

// NewRAGIndexer 创建一个新的 RAG 索引器实例
func NewRAGIndexer(sessionID, embeddingModel string) (*RAGIndexer, error) {
	ctx := context.Background()

	apiKey := os.Getenv("RAG_OPENAI_API_KEY")
	vectorSize := uint64(config.GetConfig().RagConfig.RagDimension)
	collectionName := getCollectionName()

	// 配置并创建向量生成器（embedding）
	embedConfig := &embeddingArk.EmbeddingConfig{
		BaseURL: config.GetConfig().RagConfig.RagBaseURL,
		APIKey:  apiKey,
		Model:   embeddingModel,
	}

	embedder, err := embeddingArk.NewEmbedder(ctx, embedConfig)
	if err != nil {
		mylogger.Logger.Error("Failed to create embedding instance", zap.Error(err))
		return nil, err
	}

	// 确保 Qdrant 集合存在
	if err := qdrant.CreateCollectionIfNotExists(ctx, collectionName, vectorSize); err != nil {
		mylogger.Logger.Error("Failed to ensure Qdrant collection exists", zap.Error(err))
		return nil, err
	}

	return &RAGIndexer{
		embedding:  embedder,
		client:     qdrant.Client,
		collection: collectionName,
		vectorSize: vectorSize,
	}, nil
}

// float64ToFloat32 将 float64 切片转换为 float32 切片
func float64ToFloat32(input []float64) []float32 {
	output := make([]float32, len(input))
	for i, v := range input {
		output[i] = float32(v)
	}
	return output
}

// IndexFile 读取文件内容并创建 RAG 索引
func (r *RAGIndexer) IndexFile(ctx context.Context, sessionID, filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		mylogger.Logger.Error("Failed to read file for indexing", zap.Error(err))
		return err
	}

	// 将文件内容分块处理
	docs := splitDocumentIntoChunks(string(content), filePath)

	// 准备所有点用于批量上传
	var points []*qdrantgo.PointStruct

	// 为每个文档块生成向量
	for i, doc := range docs {
		// 生成向量
		vectors, err := r.embedding.EmbedStrings(ctx, []string{doc.Content})
		if err != nil {
			mylogger.Logger.Error("Failed to generate embedding",
				zap.String("docID", doc.ID),
				zap.Error(err))
			return err
		}

		// 转换 float64 到 float32
		vectorFloat32 := float64ToFloat32(vectors[0])

		// 构建点结构 - 使用数字 ID（基于索引）
		point := &qdrantgo.PointStruct{
			Id:      qdrantgo.NewIDNum(uint64(i + 1)), // 使用数字 ID，从 1 开始
			Vectors: qdrantgo.NewVectors(vectorFloat32...),
			Payload: qdrantgo.NewValueMap(map[string]any{
				"content":    doc.Content,
				"session_id": sessionID,
				"source":     doc.MetaData["source"],
				"chunk":      doc.MetaData["chunk"],
			}),
		}
		points = append(points, point)
	}

	// 批量存储到 Qdrant
	_, err = r.client.Upsert(ctx, &qdrantgo.UpsertPoints{
		CollectionName: r.collection,
		Points:         points,
	})

	if err != nil {
		mylogger.Logger.Error("Failed to upsert points to Qdrant", zap.Error(err))
		return err
	}

	mylogger.Logger.Info("Document indexed successfully",
		zap.String("sessionID", sessionID),
		zap.Int("chunks", len(docs)))

	return nil
}

// splitDocumentIntoChunks 将文档分块处理
func splitDocumentIntoChunks(content string, filePath string) []*schema.Document {
	const maxChunkSize = 8000 // 留一些余量，避免边界问题

	var docs []*schema.Document

	// 如果内容不超过最大长度，直接作为一个文档
	if len(content) <= maxChunkSize {
		id, _ := utils.GenerateShortID(10)
		return []*schema.Document{{
			ID:       id,
			Content:  content,
			MetaData: map[string]any{"source": filePath, "chunk": 0},
		}}
	}

	// 按段落分割
	paragraphs := strings.Split(content, "\n\n")
	if len(paragraphs) == 1 {
		paragraphs = strings.Split(content, "\n")
	}

	var currentChunk strings.Builder
	chunkIndex := 0

	for _, para := range paragraphs {
		if len(para) > maxChunkSize {
			if currentChunk.Len() > 0 {
				id, _ := utils.GenerateShortID(10)
				docs = append(docs, &schema.Document{
					ID:       id,
					Content:  currentChunk.String(),
					MetaData: map[string]any{"source": filePath, "chunk": chunkIndex},
				})
				currentChunk.Reset()
				chunkIndex++
			}

			for i := 0; i < len(para); i += maxChunkSize {
				end := i + maxChunkSize
				if end > len(para) {
					end = len(para)
				}
				id, _ := utils.GenerateShortID(10)
				docs = append(docs, &schema.Document{
					ID:       id,
					Content:  para[i:end],
					MetaData: map[string]any{"source": filePath, "chunk": chunkIndex},
				})
				chunkIndex++
			}
			continue
		}

		if currentChunk.Len()+len(para)+2 > maxChunkSize {
			if currentChunk.Len() > 0 {
				id, _ := utils.GenerateShortID(10)
				docs = append(docs, &schema.Document{
					ID:       id,
					Content:  currentChunk.String(),
					MetaData: map[string]any{"source": filePath, "chunk": chunkIndex},
				})
				currentChunk.Reset()
				chunkIndex++
			}
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
		}
		currentChunk.WriteString(para)
	}

	// 保存最后一块
	if currentChunk.Len() > 0 {
		id, _ := utils.GenerateShortID(10)
		docs = append(docs, &schema.Document{
			ID:       id,
			Content:  currentChunk.String(),
			MetaData: map[string]any{"source": filePath, "chunk": chunkIndex},
		})
	}

	return docs
}

// DeleteSessionPoints 删除指定 session 的所有向量点
func DeleteSessionPoints(ctx context.Context, sessionID string) error {
	collectionName := getCollectionName()

	// 使用过滤器删除该 session 的所有点
	_, err := qdrant.Client.Delete(ctx, &qdrantgo.DeletePoints{
		CollectionName: collectionName,
		Points:         qdrantgo.NewPointsSelectorFilter(&qdrantgo.Filter{
			Must: []*qdrantgo.Condition{
				qdrantgo.NewMatch("session_id", sessionID),
			},
		}),
	})

	if err != nil {
		mylogger.Logger.Error("Failed to delete session points",
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return err
	}

	mylogger.Logger.Info("Session points deleted successfully",
		zap.String("sessionID", sessionID))
	return nil
}

// NewRAGQuery 创建一个新的 RAG 查询实例
func NewRAGQuery(ctx context.Context, sessionID string) (*RAGQuery, error) {
	cfg := config.GetConfig()
	apiKey := os.Getenv("RAG_OPENAI_API_KEY")

	embedConfig := &embeddingArk.EmbeddingConfig{
		BaseURL: cfg.RagConfig.RagBaseURL,
		APIKey:  apiKey,
		Model:   cfg.RagConfig.RagEmbeddingModel,
	}
	embedder, err := embeddingArk.NewEmbedder(ctx, embedConfig)
	if err != nil {
		mylogger.Logger.Error("Failed to create embedding instance for query", zap.Error(err))
		return nil, err
	}

	collectionName := getCollectionName()

	return &RAGQuery{
		embedding:  embedder,
		client:     qdrant.Client,
		collection: collectionName,
		sessionID:  sessionID,
	}, nil
}

// RetrieveDocuments 根据查询语句检索相关文档
func (r *RAGQuery) RetrieveDocuments(ctx context.Context, query string) ([]*schema.Document, error) {
	// 为查询生成向量
	vectors, err := r.embedding.EmbedStrings(ctx, []string{query})
	if err != nil {
		mylogger.Logger.Error("Failed to generate query embedding", zap.Error(err))
		return nil, err
	}

	// 转换 float64 到 float32
	vectorFloat32 := float64ToFloat32(vectors[0])

	// 在 Qdrant 中搜索，过滤出当前 session 的文档
	searchResult, err := r.client.Query(ctx, &qdrantgo.QueryPoints{
		CollectionName: r.collection,
		Query:          qdrantgo.NewQuery(vectorFloat32...),
		Filter: &qdrantgo.Filter{
			Must: []*qdrantgo.Condition{
				qdrantgo.NewMatch("session_id", r.sessionID),
			},
		},
		WithPayload: qdrantgo.NewWithPayload(true),
		Limit:       qdrantgo.PtrOf(uint64(5)),
	})

	if err != nil {
		mylogger.Logger.Error("Failed to query Qdrant", zap.Error(err))
		return nil, err
	}

	// 转换搜索结果为 Document
	var docs []*schema.Document
	for _, point := range searchResult {
		doc := &schema.Document{
			ID:       "",
			Content:  "",
			MetaData: map[string]any{},
		}

		// 获取 ID
		if point.Id != nil {
			doc.ID = point.Id.GetUuid()
		}

		// 提取 payload
		if point.Payload != nil {
			if content, ok := point.Payload["content"]; ok {
				if strVal := content.GetStringValue(); strVal != "" {
					doc.Content = strVal
				}
			}
			if source, ok := point.Payload["source"]; ok {
				if strVal := source.GetStringValue(); strVal != "" {
					doc.MetaData["source"] = strVal
				}
			}
			if chunk, ok := point.Payload["chunk"]; ok {
				doc.MetaData["chunk"] = chunk.GetIntegerValue()
			}
		}

		// 添加相似度分数
		doc.MetaData["score"] = point.Score

		docs = append(docs, doc)
	}

	mylogger.Logger.Info("Documents retrieved successfully",
		zap.String("sessionID", r.sessionID),
		zap.Int("count", len(docs)))

	return docs, nil
}

// BuildRagPrompt 构建 RAG 提示语
func BuildRagPrompt(query string, docs []*schema.Document) string {
	if len(docs) == 0 {
		return query
	}

	var contextBuilder strings.Builder
	for i, doc := range docs {
		contextBuilder.WriteString(fmt.Sprintf("【参考文档 %d】\n%s\n\n", i+1, doc.Content))
	}

	prompt := fmt.Sprintf(`以下是用户上传的参考文档，你可以参考这些内容来回答问题：

%s

用户问题：%s

请注意：
1. 如果问题与参考文档相关，请结合文档内容给出详细、专业的回答
2. 如果问题超出参考文档的范围，你可以根据你的知识灵活回答，不必局限于文档
3. 回答要自然流畅，像正常对话一样，不要公式化
4. 如果参考文档有帮助，可以适当提及"根据你上传的文档..."，但不要生硬`, contextBuilder.String(), query)

	return prompt
}
