package models

import "gorm.io/gorm"

// Like 记录用户与文章的点赞关联（持久化存储）
type Like struct {
	gorm.Model
	UserID       uint  `gorm:"uniqueIndex:idx_user_article"`
	ArticleID    uint  `gorm:"uniqueIndex:idx_user_article;index:idx_likes_article"`
	StateVersion int64 `gorm:"not null;default:0"`
}
