package models

import (
	"time"

	"gorm.io/gorm"
)

type Article struct {
	gorm.Model
	Title      string `binding:"required"`
	Content    string `binding:"required"`
	Preview    string `binding:"required"`
	UserID     uint   `json:"user_id"`                      // 作者ID
	Username   string `json:"username"`                     // 作者用户名（冗余存储，方便查询）
	LikesCount int64  `json:"likes_count" gorm:"default:0"` // 点赞汇总
	RagStatus  string `json:"rag_status" gorm:"type:varchar(20);default:'pending';index"`
	RagVersion int    `json:"rag_version" gorm:"default:1"`
	// RagIndexSchemaVersion tracks the persisted chunk/metadata layout separately
	// from the article content version. It lets a splitter upgrade trigger a safe
	// re-index even when the article body itself has not changed.
	RagIndexSchemaVersion int        `json:"rag_index_schema_version" gorm:"default:1;index"`
	RagIndexedAt          *time.Time `json:"rag_indexed_at"`
	RagError              string     `json:"rag_error" gorm:"type:text"`
	ContentHash           string     `json:"content_hash" gorm:"type:varchar(64);index"`
}
