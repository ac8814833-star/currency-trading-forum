package utils

import (
	"back/global"
	"back/models"
	"errors"
	"strings"

	"github.com/sirupsen/logrus"
)

// ArticleSearchResult 文章搜索返回
type ArticleSearchResult struct {
	Articles []models.Article `json:"articles"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	Source   string           `json:"source"` // elasticsearch | mysql
}

// SearchArticles 按关键词搜索文章：优先 Elasticsearch，失败时降级 MySQL FULLTEXT/LIKE
func SearchArticles(keyword string, page, pageSize int) (*ArticleSearchResult, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, errors.New("keyword is required")
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 50 {
		pageSize = 50
	}

	if global.ESClient != nil {
		result, err := searchArticlesByES(keyword, page, pageSize)
		if err == nil {
			return result, nil
		}
		logrus.Warnf("ES article search failed, fallback to MySQL: %v", err)
	}

	return searchArticlesByMySQL(keyword, page, pageSize)
}

func searchArticlesByES(keyword string, page, pageSize int) (*ArticleSearchResult, error) {
	hits, total, err := ESBM25SearchPage(keyword, (page-1)*pageSize, pageSize)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return &ArticleSearchResult{
			Articles: []models.Article{},
			Total:    total,
			Page:     page,
			PageSize: pageSize,
			Source:   "elasticsearch",
		}, nil
	}

	ids := make([]uint, 0, len(hits))
	for _, h := range hits {
		if h.ArticleID > 0 {
			ids = append(ids, h.ArticleID)
		}
	}

	articles, err := loadAliveArticlesOrdered(ids)
	if err != nil {
		return nil, err
	}

	if len(articles) != len(ids) {
		return nil, errors.New("stale ES article index")
	}

	return &ArticleSearchResult{
		Articles: articles,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Source:   "elasticsearch",
	}, nil
}

func searchArticlesByMySQL(keyword string, page, pageSize int) (*ArticleSearchResult, error) {
	if global.Db == nil {
		return nil, errors.New("db is not initialized")
	}

	offset := (page - 1) * pageSize
	var articles []models.Article
	var total int64

	if ensureArticleFullTextIndex() {
		countSQL := "SELECT COUNT(1) FROM articles WHERE MATCH(title, preview, content) AGAINST (? IN NATURAL LANGUAGE MODE) AND deleted_at IS NULL"
		if err := global.Db.Raw(countSQL, keyword).Scan(&total).Error; err != nil {
			return searchArticlesByLike(keyword, page, pageSize)
		}
		listSQL := "SELECT * FROM articles WHERE MATCH(title, preview, content) AGAINST (? IN NATURAL LANGUAGE MODE) AND deleted_at IS NULL ORDER BY MATCH(title, preview, content) AGAINST (? IN NATURAL LANGUAGE MODE) DESC LIMIT ? OFFSET ?"
		if err := global.Db.Raw(listSQL, keyword, keyword, pageSize, offset).Scan(&articles).Error; err != nil {
			return searchArticlesByLike(keyword, page, pageSize)
		}
		if articles == nil {
			articles = []models.Article{}
		}
		return &ArticleSearchResult{
			Articles: articles,
			Total:    int(total),
			Page:     page,
			PageSize: pageSize,
			Source:   "mysql",
		}, nil
	}

	return searchArticlesByLike(keyword, page, pageSize)
}

func searchArticlesByLike(keyword string, page, pageSize int) (*ArticleSearchResult, error) {
	like := "%" + keyword + "%"
	offset := (page - 1) * pageSize

	var total int64
	q := global.Db.Model(&models.Article{}).
		Where("title LIKE ? OR preview LIKE ? OR content LIKE ?", like, like, like)
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}

	var articles []models.Article
	if err := q.Order("id DESC").Limit(pageSize).Offset(offset).Find(&articles).Error; err != nil {
		return nil, err
	}
	if articles == nil {
		articles = []models.Article{}
	}

	return &ArticleSearchResult{
		Articles: articles,
		Total:    int(total),
		Page:     page,
		PageSize: pageSize,
		Source:   "mysql",
	}, nil
}

func loadAliveArticlesOrdered(ids []uint) ([]models.Article, error) {
	if len(ids) == 0 {
		return []models.Article{}, nil
	}
	if global.Db == nil {
		return nil, errors.New("db is not initialized")
	}

	var articles []models.Article
	if err := global.Db.Where("id IN ?", ids).Find(&articles).Error; err != nil {
		return nil, err
	}

	m := make(map[uint]models.Article, len(articles))
	for _, a := range articles {
		m[a.ID] = a
	}

	ordered := make([]models.Article, 0, len(ids))
	for _, id := range ids {
		if a, ok := m[id]; ok {
			ordered = append(ordered, a)
		}
	}
	return ordered, nil
}
