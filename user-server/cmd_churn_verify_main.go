package main

import (
	"context"
	"fmt"
	"os"

	"hivemtk-user/internal/ops/model"
	"hivemtk-user/internal/ops/service"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	dsn := os.Getenv("CHURN_VERIFY_DSN")
	if dsn == "" {
		dsn = "host=localhost port=8232 user=admin password=mtk2024 dbname=user_db sslmode=disable TimeZone=Asia/Shanghai"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Println("DB_OPEN_ERR:", err)
		os.Exit(2)
	}
	// 确保 churn 表存在（避免首次运行时缺表）
	_ = db.AutoMigrate(&model.ChurnPrediction{}, &model.ChurnWarning{}, &model.ChurnStatistics{}, &model.ChurnModelConfig{})

	svc := service.NewChurnPredictionServiceWithDB(db)
	n, err := svc.RunChurnCalculationForAllCustomers(context.Background())
	if err != nil {
		fmt.Println("CALC_ERR:", err)
		os.Exit(3)
	}
	fmt.Printf("COVERED_CUSTOMERS=%d\n", n)

	var predCount int64
	db.Model(&model.ChurnPrediction{}).Count(&predCount)
	var warnCount int64
	db.Model(&model.ChurnWarning{}).Count(&warnCount)
	var statCount int64
	db.Model(&model.ChurnStatistics{}).Count(&statCount)
	fmt.Printf("churn_predictions=%d churn_warnings=%d churn_statistics=%d\n", predCount, warnCount, statCount)

	// 分布
	type dist struct {
		Risk  string
		Cnt   int64
	}
	var dists []dist
	db.Model(&model.ChurnPrediction{}).Select("churn_risk as risk, count(*) as cnt").Group("churn_risk").Scan(&dists)
	for _, d := range dists {
		fmt.Printf("risk=%s count=%d\n", d.Risk, d.Cnt)
	}
}
