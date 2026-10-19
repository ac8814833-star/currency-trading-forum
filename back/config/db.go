package config

import (
	"back/global"
	"back/models"
	"log"
	"time"

	"github.com/sirupsen/logrus"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func initDB() {
	dsn := AppConfig.Database.DSN
	var (
		db  *gorm.DB
		err error
	)
	deadline := time.Now().Add(60 * time.Second)
	for {
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			logrus.Fatalf("Failed to initialize database connection: %v", err)
		}
		time.Sleep(2 * time.Second)
	}
	sqlDB, err := db.DB()
	if err != nil {
		logrus.Fatalf("Failed to configure database connection: %v", err)
	}
	sqlDB.SetMaxIdleConns(AppConfig.Database.MaxIdleConns)
	sqlDB.SetMaxOpenConns(AppConfig.Database.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Hour)
	global.Db = db

	// 自动迁移数据库表结构（确保 users、articles、comments、exchange_rates、likes 等表存在）
	deadline = time.Now().Add(60 * time.Second)
	for {
		if err := db.AutoMigrate(
			&models.User{},
			&models.Article{},
			&models.Comment{},
			&models.ExchangeRate{},
			&models.Like{},
		); err != nil {
			if time.Now().After(deadline) {
				log.Fatalf("Failed to auto migrate database: %v", err)
			}
			time.Sleep(2 * time.Second)
			continue
		}
		break
	}
	seedExchangeRates(db)

	// 初始化 Elasticsearch（后台异步，不影响启动）
	go InitElasticsearch()
}

// CurrencyNameMap 货币代码 → 中文名称映射，供汇率接口使用
var CurrencyNameMap = map[string]string{
	"USD": "美元",
	"EUR": "欧元",
	"GBP": "英镑",
	"JPY": "日元",
	"HKD": "港币",
	"AUD": "澳元",
	"CAD": "加元",
	"CHF": "瑞郎",
	"SGD": "新加坡元",
	"KRW": "韩元",
	"CNY": "人民币",
}

func seedExchangeRates(db *gorm.DB) {
	var count int64
	db.Model(&models.ExchangeRate{}).Count(&count)
	if count == 0 {
		rates := []models.ExchangeRate{
			{FromCurrency: "USD", ToCurrency: "CNY", Rate: 7.25, Date: time.Now()},
			{FromCurrency: "EUR", ToCurrency: "CNY", Rate: 7.85, Date: time.Now()},
			{FromCurrency: "GBP", ToCurrency: "CNY", Rate: 9.15, Date: time.Now()},
			{FromCurrency: "JPY", ToCurrency: "CNY", Rate: 0.048, Date: time.Now()},
			{FromCurrency: "HKD", ToCurrency: "CNY", Rate: 0.93, Date: time.Now()},
			{FromCurrency: "AUD", ToCurrency: "CNY", Rate: 4.78, Date: time.Now()},
			{FromCurrency: "CAD", ToCurrency: "CNY", Rate: 5.32, Date: time.Now()},
			{FromCurrency: "CHF", ToCurrency: "CNY", Rate: 8.05, Date: time.Now()},
			{FromCurrency: "SGD", ToCurrency: "CNY", Rate: 5.42, Date: time.Now()},
			{FromCurrency: "KRW", ToCurrency: "CNY", Rate: 0.0054, Date: time.Now()},
			{FromCurrency: "CNY", ToCurrency: "CNY", Rate: 1.0, Date: time.Now()},
		}
		if err := db.Create(&rates).Error; err != nil {
			log.Printf("Failed to seed exchange rates: %v", err)
		} else {
			log.Println("Seeded exchange rates successfully")
		}
	}
}
