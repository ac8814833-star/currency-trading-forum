package utils

import (
	"back/config"
	"back/global"
	"back/models"
	"time"

	"github.com/sirupsen/logrus"
)

func StartRAGSyncWorker() {
	if config.AppConfig == nil {
		return
	}
	if !config.AppConfig.RAG.AutoIndex {
		return
	}
	if config.AppConfig.RAG.SyncIntervalSecond <= 0 {
		return
	}

	go func() {
		if config.AppConfig.RAG.FullSyncOnStart {
			fullSyncRAG()
		}

		lastUpdated := time.Now()
		ticker := time.NewTicker(time.Duration(config.AppConfig.RAG.SyncIntervalSecond) * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			incrementalSyncRAG(&lastUpdated)
		}
	}()
}

func fullSyncRAG() {
	if global.Db == nil {
		return
	}
	batch := config.AppConfig.RAG.SyncBatchSize
	if batch <= 0 {
		batch = 50
	}

	cleanupDeletedRAG(time.Time{}, time.Now(), batch)
	dispatchPendingRAG(batch)
}

func incrementalSyncRAG(lastUpdated *time.Time) {
	if global.Db == nil {
		return
	}
	batch := config.AppConfig.RAG.SyncBatchSize
	if batch <= 0 {
		batch = 50
	}

	windowEnd := time.Now()
	dispatchPendingRAG(batch)
	cleanupDeletedRAG(*lastUpdated, windowEnd, batch)
	*lastUpdated = windowEnd
}

func dispatchPendingRAG(batch int) {
	var lastID uint

	for {
		var articles []models.Article
		err := global.Db.
			Where("id > ?", lastID).
			Where("deleted_at IS NULL").
			Where("rag_status IN ? OR content_hash = '' OR content_hash IS NULL OR (rag_status = ? AND rag_indexed_at IS NULL) OR rag_index_schema_version <> ?", []string{RAGStatusPending, RAGStatusFailed}, RAGStatusIndexed, CurrentRAGIndexSchemaVersion()).
			Order("id asc").
			Limit(batch).
			Find(&articles).Error
		if err != nil {
			logrus.Errorf("rag pending sync query failed: %v", err)
			return
		}
		if len(articles) == 0 {
			return
		}

		for _, a := range articles {
			if err := AsyncIndexArticleByID(a.ID, a.RagVersion); err != nil {
				// 仅记录日志，保持 pending 状态，下一轮再重试
				logrus.Warnf("rag pending sync dispatch failed (kept pending): article=%d err=%v", a.ID, err)
			}
			lastID = a.ID
		}
	}
}

func cleanupDeletedRAG(since time.Time, until time.Time, batch int) {
	cursorTime := since
	cursorID := ^uint(0)

	for {
		var articles []models.Article
		err := global.Db.
			Unscoped().
			Where("deleted_at IS NOT NULL").
			Where("deleted_at <= ?", until).
			Where("(deleted_at > ?) OR (deleted_at = ? AND id > ?)", cursorTime, cursorTime, cursorID).
			Order("deleted_at asc, id asc").
			Limit(batch).
			Find(&articles).Error
		if err != nil {
			logrus.Errorf("rag cleanup deleted query failed: %v", err)
			return
		}
		if len(articles) == 0 {
			return
		}

		for _, a := range articles {
			if err := DeleteArticleChunks(a.ID); err != nil {
				logrus.Errorf("rag cleanup deleted vectors failed: %v", err)
			}
			cursorTime = a.DeletedAt.Time
			cursorID = a.ID
		}
	}
}
