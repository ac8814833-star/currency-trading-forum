package utils

import (
	"back/config"
	"back/models"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"
)

type ArticleIndexChunk struct {
	IDSuffix      string
	Content       string
	DocType       string
	ChunkIndex    int
	ParentID      string
	ParentContent string
}

func BuildArticleDocument(article models.Article) string {
	var builder strings.Builder

	if title := strings.TrimSpace(article.Title); title != "" {
		builder.WriteString(fmt.Sprintf("标题：%s\n", title))
	}
	if preview := strings.TrimSpace(article.Preview); preview != "" {
		builder.WriteString(fmt.Sprintf("摘要：%s\n", preview))
	}
	if username := strings.TrimSpace(article.Username); username != "" {
		builder.WriteString(fmt.Sprintf("作者：%s\n", username))
	}

	content := strings.TrimSpace(article.Content)
	if content != "" {
		builder.WriteString("正文：\n")
		builder.WriteString(content)
	}

	return strings.TrimSpace(builder.String())
}

func buildArticleContextHeader(article models.Article) string {
	var builder strings.Builder

	if title := strings.TrimSpace(article.Title); title != "" {
		builder.WriteString(fmt.Sprintf("标题：%s\n", title))
	}
	if preview := strings.TrimSpace(article.Preview); preview != "" {
		builder.WriteString(fmt.Sprintf("摘要：%s\n", preview))
	}
	if username := strings.TrimSpace(article.Username); username != "" {
		builder.WriteString(fmt.Sprintf("作者：%s\n", username))
	}

	return strings.TrimSpace(builder.String())
}

func BuildArticleIndexHash(article models.Article) string {
	source := strings.TrimSpace(strings.Join([]string{
		article.Title,
		article.Preview,
		article.Content,
	}, "\n"))
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

func BuildArticleIndexChunks(article models.Article) []ArticleIndexChunk {
	if config.AppConfig != nil && config.AppConfig.RAG.ParentChildEnabled {
		return buildParentChildChunks(article)
	}
	return buildFlatArticleChunks(article)
}

func buildFlatArticleChunks(article models.Article) []ArticleIndexChunk {
	document := BuildArticleDocument(article)
	if document == "" {
		return nil
	}

	chunkSize := 400
	overlap := 80
	shortArticleThreshold := 800

	if config.AppConfig != nil {
		if config.AppConfig.RAG.ChunkSize > 0 {
			chunkSize = config.AppConfig.RAG.ChunkSize
		}
		if config.AppConfig.RAG.ChunkOverlap >= 0 {
			overlap = config.AppConfig.RAG.ChunkOverlap
		}
		if config.AppConfig.RAG.ShortArticleThreshold > 0 {
			shortArticleThreshold = config.AppConfig.RAG.ShortArticleThreshold
		}
	}

	if utf8.RuneCountInString(document) <= shortArticleThreshold {
		return []ArticleIndexChunk{
			{
				IDSuffix:   "chunk_0",
				Content:    document,
				DocType:    "child",
				ChunkIndex: 0,
			},
		}
	}

	texts := ChunkText(document, chunkSize, overlap)
	chunks := make([]ArticleIndexChunk, 0, len(texts))
	for i, text := range texts {
		chunks = append(chunks, ArticleIndexChunk{
			IDSuffix:   fmt.Sprintf("chunk_%d", i),
			Content:    text,
			DocType:    "child",
			ChunkIndex: i,
		})
	}
	return chunks
}

// buildParentChildChunks creates real local parent segments. Parent chunks are
// not inserted into the vector index: only their child chunks are searchable,
// while each child carries a stable parent_id and the corresponding parent text
// for small-to-big context reconstruction.
func buildParentChildChunks(article models.Article) []ArticleIndexChunk {
	content := strings.TrimSpace(article.Content)
	header := buildArticleContextHeader(article)
	if content == "" {
		if header == "" {
			return nil
		}
		return []ArticleIndexChunk{{
			IDSuffix:      "child_0_0",
			Content:       header,
			DocType:       "child",
			ChunkIndex:    0,
			ParentID:      stableParentID(article, 0),
			ParentContent: header,
		}}
	}

	childSize, childOverlap, parentSize, parentOverlap := articleChunkSizes()
	parentBodies := ChunkText(content, parentSize, parentOverlap)
	if len(parentBodies) == 0 {
		parentBodies = []string{content}
	}

	chunks := make([]ArticleIndexChunk, 0, len(parentBodies)*2)
	globalChildIndex := 0
	for parentIndex, parentBody := range parentBodies {
		parentID := stableParentID(article, parentIndex)
		parentContent := joinArticleContext(header, parentBody)
		childTexts := ChunkText(parentBody, childSize, childOverlap)
		if len(childTexts) == 0 {
			childTexts = []string{parentBody}
		}

		for childIndex, childText := range childTexts {
			chunks = append(chunks, ArticleIndexChunk{
				IDSuffix:      fmt.Sprintf("child_%d_%d", parentIndex, childIndex),
				Content:       joinArticleContext(header, childText),
				DocType:       "child",
				ChunkIndex:    globalChildIndex,
				ParentID:      parentID,
				ParentContent: parentContent,
			})
			globalChildIndex++
		}
	}
	return chunks
}

func articleChunkSizes() (childSize, childOverlap, parentSize, parentOverlap int) {
	childSize, childOverlap = 400, 80
	parentSize, parentOverlap = 1200, 160
	if config.AppConfig == nil {
		return
	}
	if config.AppConfig.RAG.ChunkSize > 0 {
		childSize = config.AppConfig.RAG.ChunkSize
	}
	if config.AppConfig.RAG.ChunkOverlap >= 0 {
		childOverlap = config.AppConfig.RAG.ChunkOverlap
	}
	if config.AppConfig.RAG.ParentChunkSize > 0 {
		parentSize = config.AppConfig.RAG.ParentChunkSize
	}
	if config.AppConfig.RAG.ParentChunkOverlap >= 0 {
		parentOverlap = config.AppConfig.RAG.ParentChunkOverlap
	}
	if parentSize < childSize {
		parentSize = childSize
	}
	if parentOverlap >= parentSize || parentOverlap < 0 {
		parentOverlap = 0
	}
	if childOverlap >= childSize || childOverlap < 0 {
		childOverlap = 0
	}
	return
}

func stableParentID(article models.Article, parentIndex int) string {
	version := article.RagVersion
	if version <= 0 {
		version = 1
	}
	return fmt.Sprintf("article_%d_v%d_parent_%d", article.ID, version, parentIndex)
}

func joinArticleContext(header, body string) string {
	header = strings.TrimSpace(header)
	body = strings.TrimSpace(body)
	switch {
	case header == "":
		return body
	case body == "":
		return header
	default:
		return header + "\n正文：\n" + body
	}
}

func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimSpace(string(runes[:limit]))
}
