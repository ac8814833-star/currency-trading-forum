package controllers

import (
	"back/config"
	"back/global"
	"back/models"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// GET /api/currency/rates 返回格式化汇率列表
func GetCurrencyRates(c *gin.Context) {
	var rates []models.ExchangeRate
	if err := global.Db.Where("to_currency = 'CNY'").Order("date DESC, id DESC").Find(&rates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	codeToName := config.CurrencyNameMap

	type RateItem struct {
		Currency  string  `json:"currency"`
		Name      string  `json:"name"`
		Rate      float64 `json:"rate"`
		Timestamp string  `json:"timestamp"`
	}

	items := make([]RateItem, 0, len(rates))
	seen := make(map[string]bool, len(rates))
	for _, r := range rates {
		if seen[r.FromCurrency] || r.Rate <= 0 || math.IsNaN(r.Rate) || math.IsInf(r.Rate, 0) {
			continue
		}
		seen[r.FromCurrency] = true
		items = append(items, RateItem{
			Currency:  r.FromCurrency,
			Name:      codeToName[r.FromCurrency],
			Rate:      r.Rate,
			Timestamp: r.Date.Format("2006-01-02 15:04:05"),
		})
	}

	c.JSON(http.StatusOK, items)
}

// GET /api/currency/convert?from=USD&to=CNY&amount=100
func ConvertCurrency(c *gin.Context) {
	from := c.Query("from")
	to := c.Query("to")
	amountStr := c.Query("amount")

	if from == "" {
		from = "USD"
	}
	if to == "" {
		to = "CNY"
	}

	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid amount"})
		return
	}

	from = normalizeCode(from)
	to = normalizeCode(to)
	if _, ok := config.CurrencyNameMap[from]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported currency: " + from})
		return
	}
	if _, ok := config.CurrencyNameMap[to]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported currency: " + to})
		return
	}

	if from == to {
		c.JSON(http.StatusOK, gin.H{
			"from":      from,
			"to":        to,
			"amount":    amount,
			"converted": fmt.Sprintf("%.2f", amount),
			"rate":      1.0,
			"time":      time.Now().Format("2006-01-02 15:04:05"),
		})
		return
	}

	// 对 CNY 的汇率
	var fromRate float64
	var toRate float64
	rateTime := time.Now()

	if from == "CNY" {
		fromRate = 1.0
	} else {
		var er models.ExchangeRate
		if err := global.Db.Where("from_currency = ? AND to_currency = 'CNY'", from).Order("date DESC, id DESC").First(&er).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("unsupported currency: %s", from)})
			return
		}
		fromRate = er.Rate
		rateTime = er.Date
	}

	if to == "CNY" {
		toRate = 1.0
	} else {
		var er models.ExchangeRate
		if err := global.Db.Where("from_currency = ? AND to_currency = 'CNY'", to).Order("date DESC, id DESC").First(&er).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("unsupported currency: %s", to)})
			return
		}
		toRate = er.Rate
		if from == "CNY" || er.Date.Before(rateTime) {
			rateTime = er.Date
		}
	}
	if fromRate <= 0 || toRate <= 0 || math.IsNaN(fromRate/toRate) || math.IsInf(fromRate/toRate, 0) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "rate data unavailable"})
		return
	}

	rate := fromRate / toRate
	result := math.Round(amount*rate*100) / 100

	c.JSON(http.StatusOK, gin.H{
		"from":      from,
		"to":        to,
		"amount":    amount,
		"converted": fmt.Sprintf("%.2f", result),
		"rate":      math.Round(rate*10000) / 10000,
		"time":      rateTime.Format("2006-01-02 15:04:05"),
	})
}

func normalizeCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "RMB" {
		return "CNY"
	}
	return code
}
