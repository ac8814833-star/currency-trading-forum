package models

import "time"

type ExchangeRate struct {
	ID           uint      `gorm:"primarykey" json:"_id"`
	FromCurrency string    `json:"fromCurrency" binding:"required" gorm:"index:idx_rate_pair_date,priority:1"`
	ToCurrency   string    `json:"toCurrency" binding:"required" gorm:"index:idx_rate_pair_date,priority:2"`
	Rate         float64   `json:"rate" binding:"required"`
	Date         time.Time `json:"date" gorm:"index:idx_rate_pair_date,priority:3"`
}
