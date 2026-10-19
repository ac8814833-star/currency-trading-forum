package config

import (
	"back/global"
	"back/models"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/sirupsen/logrus"
)

func InitElasticsearch() {
	if AppConfig.RAG.ElasticsearchURL == "" {
		logrus.Warn("elasticsearch url is empty, ES features disabled")
		return
	}

	cfg := elasticsearch.Config{
		Addresses:     []string{AppConfig.RAG.ElasticsearchURL},
		RetryOnStatus: []int{429, 502, 503, 504},
		MaxRetries:    5,
	}

	deadline := time.Now().Add(60 * time.Second)
	var err error
	for {
		global.ESClient, err = elasticsearch.NewClient(cfg)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			logrus.Warnf("Failed to create ES client: %v (ES features disabled)", err)
			return
		}
		time.Sleep(2 * time.Second)
	}

	for {
		resp, pingErr := global.ESClient.Ping()
		if pingErr == nil && resp != nil && !resp.IsError() {
			if resp.Body != nil {
				resp.Body.Close()
			}
			break
		}
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if time.Now().After(deadline) {
			logrus.Warn("ES ping failed, ES features disabled")
			global.ESClient = nil
			return
		}
		time.Sleep(2 * time.Second)
	}

	if err := ensureESIndex(); err != nil {
		logrus.Warnf("ES index creation failed: %v", err)
		global.ESClient = nil
		return
	}

	// 全量同步已有文章到 ES（异步）
	go syncExistingArticlesToES()

	logrus.Info("Elasticsearch connected, articles index ready")
}

const ArticlesIndex = "forum_articles"

func ensureESIndex() error {
	resp, err := global.ESClient.Indices.Exists([]string{ArticlesIndex})
	if err != nil {
		return fmt.Errorf("check index exists: %w", err)
	}
	if resp.Body != nil {
		resp.Body.Close()
	}
	if resp.StatusCode == 200 {
		return nil
	}

	body := `{
		"settings": {
			"number_of_shards": 1,
			"number_of_replicas": 0,
			"analysis": {
				"analyzer": {
					"zh_analyzer": {
						"type": "standard"
					}
				}
			}
		},
		"mappings": {
			"properties": {
				"article_id":  { "type": "long" },
				"title":       { "type": "text", "analyzer": "zh_analyzer" },
				"preview":     { "type": "text", "analyzer": "zh_analyzer" },
				"content":     { "type": "text", "analyzer": "zh_analyzer" }
			}
		}
	}`

	resp2, err := global.ESClient.Indices.Create(
		ArticlesIndex,
		global.ESClient.Indices.Create.WithBody(strings.NewReader(body)),
	)
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}
	if resp2.Body != nil {
		resp2.Body.Close()
	}
	return nil
}

func CallInitElasticsearchFromDB() {
	InitElasticsearch()
}

// syncExistingArticlesToES 全量同步现有文章到 Elasticsearch
func syncExistingArticlesToES() {
	const batchSize = 20
	var lastID uint
	indexed := 0

	for {
		var articles []models.Article
		if err := global.Db.
			Where("id > ? AND deleted_at IS NULL", lastID).
			Order("id asc").
			Limit(batchSize).
			Find(&articles).Error; err != nil {
			logrus.Warnf("ES sync: query articles failed: %v", err)
			return
		}
		if len(articles) == 0 {
			break
		}

		for _, a := range articles {
			doc := map[string]interface{}{
				"article_id": a.ID,
				"title":      a.Title,
				"preview":    a.Preview,
				"content":    a.Content,
			}
			body, _ := json.Marshal(doc)
			resp, err := global.ESClient.Index(
				ArticlesIndex,
				bytes.NewReader(body),
				global.ESClient.Index.WithDocumentID(fmt.Sprintf("%d", a.ID)),
				global.ESClient.Index.WithRefresh("false"),
			)
			if err != nil {
				logrus.Warnf("ES sync: index article %d failed: %v", a.ID, err)
			} else {
				if resp.Body != nil {
					resp.Body.Close()
				}
				indexed++
			}
			lastID = a.ID
		}

		if len(articles) < batchSize {
			break
		}
	}

	logrus.Infof("ES sync: indexed %d articles", indexed)
}
