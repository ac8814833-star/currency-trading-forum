package utils

import (
	"back/config"
	"back/global"
	"time"
)

const (
	RAGStatusPending             = "pending"
	RAGStatusIndexed             = "indexed"
	RAGStatusFailed              = "failed"
	defaultRAGIndexSchemaVersion = 2
)

// CurrentRAGIndexSchemaVersion is intentionally separate from RagVersion:
// RagVersion changes with article content, while this value changes when the
// persisted chunk format or RAG metadata layout changes.
func CurrentRAGIndexSchemaVersion() int {
	if config.AppConfig != nil && config.AppConfig.RAG.IndexSchemaVersion > 0 {
		return config.AppConfig.RAG.IndexSchemaVersion
	}
	return defaultRAGIndexSchemaVersion
}

func MarkArticleRAGPending(articleID uint, ragVersion int, contentHash string) {
	if global.Db == nil || articleID == 0 {
		return
	}

	updates := map[string]interface{}{
		"rag_status":   RAGStatusPending,
		"rag_error":    "",
		"content_hash": contentHash,
	}
	if ragVersion > 0 {
		updates["rag_version"] = ragVersion
	}
	if err := global.Db.Model(&struct{ ID uint }{}).Table("articles").Where("id = ?", articleID).Updates(updates).Error; err != nil {
		return
	}
	_ = global.Db.Model(&struct{ ID uint }{}).Table("articles").Where("id = ?", articleID).Update("rag_indexed_at", nil).Error
}

func MarkArticleRAGFailed(articleID uint, ragVersion int, contentHash string, err error) {
	if global.Db == nil || articleID == 0 {
		return
	}

	updates := map[string]interface{}{
		"rag_status": RAGStatusFailed,
	}
	if err != nil {
		updates["rag_error"] = err.Error()
	}

	if updateErr := global.Db.Table("articles").Where("id = ? AND rag_version = ? AND content_hash = ? AND deleted_at IS NULL", articleID, ragVersion, contentHash).Updates(updates).Error; updateErr != nil {
		return
	}
	_ = global.Db.Table("articles").Where("id = ? AND rag_version = ? AND content_hash = ? AND deleted_at IS NULL", articleID, ragVersion, contentHash).Update("rag_indexed_at", nil).Error
}

func MarkArticleRAGIndexed(articleID uint, ragVersion int, contentHash string) bool {
	if global.Db == nil || articleID == 0 {
		return false
	}

	now := time.Now()
	updates := map[string]interface{}{
		"rag_status":               RAGStatusIndexed,
		"rag_error":                "",
		"rag_indexed_at":           &now,
		"rag_index_schema_version": CurrentRAGIndexSchemaVersion(),
	}
	tx := global.Db.Table("articles").Where("id = ? AND rag_version = ? AND content_hash = ? AND deleted_at IS NULL", articleID, ragVersion, contentHash).Updates(updates)
	return tx.Error == nil && tx.RowsAffected == 1
}
