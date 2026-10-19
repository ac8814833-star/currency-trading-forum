package utils

import (
	"back/config"
	"back/global"
	"back/models"
	"context"
	"errors"
	"github.com/sirupsen/logrus"
)

func IndexArticleByID(ctx context.Context, articleID uint) error {
	if global.Db == nil {
		return errors.New("db is not initialized")
	}
	if config.AppConfig == nil {
		return errors.New("config is not initialized")
	}
	if config.AppConfig.RAG.DashScopeKey == "" {
		return errors.New("rag.dashScopeKey is empty")
	}
	if config.AppConfig.RAG.ChromaURL == "" {
		return errors.New("rag.chromaURL is empty")
	}

	var article models.Article
	if err := global.Db.WithContext(ctx).First(&article, articleID).Error; err != nil {
		return err
	}

	contentHash := BuildArticleIndexHash(article)
	if article.RagStatus == RAGStatusIndexed &&
		article.ContentHash != "" &&
		article.ContentHash == contentHash &&
		article.RagIndexSchemaVersion == CurrentRAGIndexSchemaVersion() {
		return nil
	}

	chunks := BuildArticleIndexChunks(article)
	if len(chunks) == 0 {
		err := errors.New("article document is empty")
		MarkArticleRAGFailed(article.ID, article.RagVersion, contentHash, err)
		return err
	}

	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		texts = append(texts, chunk.Content)
	}

	vectors, err := EmbedTexts(texts)
	if err != nil {
		MarkArticleRAGFailed(article.ID, article.RagVersion, contentHash, err)
		return err
	}

	if err := UpsertArticleChunks(article.ID, article.Title, article.RagVersion, chunks, vectors); err != nil {
		MarkArticleRAGFailed(article.ID, article.RagVersion, contentHash, err)
		return err
	}

	if MarkArticleRAGIndexed(article.ID, article.RagVersion, contentHash) {
		if err := DeleteOldArticleChunks(article.ID, article.RagVersion, CurrentRAGIndexSchemaVersion()); err != nil {
			logrus.Warnf("old rag chunks cleanup failed: article=%d err=%v", article.ID, err)
		}
	}

	return nil
}
