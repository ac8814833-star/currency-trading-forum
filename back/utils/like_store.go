package utils

import (
	"back/global"
	"back/models"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrLikeArticleNotFound = errors.New("article not found")
	ErrAlreadyLiked        = errors.New("already liked")
	ErrNotLiked            = errors.New("not liked")
)

// ToggleLikeInDB serializes changes to an article and commits the relation and
// aggregate together. Redis loss and MQ delivery can no longer change the answer.
func ToggleLikeInDB(articleID, userID uint, direction int) (int64, error) {
	if global.Db == nil || articleID == 0 || userID == 0 || (direction != 1 && direction != -1) {
		return 0, errors.New("invalid like request")
	}
	var count int64
	err := global.Db.Transaction(func(tx *gorm.DB) error {
		var article models.Article
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&article, articleID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrLikeArticleNotFound
			}
			return err
		}

		var like models.Like
		err := tx.Unscoped().Where("article_id = ? AND user_id = ?", articleID, userID).First(&like).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		active := err == nil && !like.DeletedAt.Valid
		if direction == 1 {
			if active {
				return ErrAlreadyLiked
			}
			if err == nil {
				if err := tx.Unscoped().Model(&like).Update("deleted_at", nil).Error; err != nil {
					return err
				}
			} else if err := tx.Create(&models.Like{ArticleID: articleID, UserID: userID}).Error; err != nil {
				return err
			}
		} else {
			if !active {
				return ErrNotLiked
			}
			if err := tx.Delete(&like).Error; err != nil {
				return err
			}
		}

		if err := tx.Model(&models.Like{}).Where("article_id = ?", articleID).Count(&count).Error; err != nil {
			return err
		}
		return tx.Model(&article).Update("likes_count", count).Error
	})
	return count, err
}
