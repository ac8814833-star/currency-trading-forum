package models

import "gorm.io/gorm"

type Comment struct {
	gorm.Model
	ArticleID uint      `json:"article_id" gorm:"index:idx_comment_article_parent"`
	UserID    uint      `json:"user_id"`
	Username  string    `json:"username"`
	Content   string    `json:"content" binding:"required"`
	ParentID  *uint     `json:"parent_id" gorm:"index:idx_comment_article_parent"` // 用于回复评论，nil表示顶级评论
	Replies   []Comment `gorm:"foreignKey:ParentID" json:"replies,omitempty"`
}
