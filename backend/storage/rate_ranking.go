package storage

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"
)

type RateRankingConfigs struct{ db *gorm.DB }

func NewRateRankingConfigs(db *gorm.DB) *RateRankingConfigs {
	return &RateRankingConfigs{db: db}
}

func (r *RateRankingConfigs) List(ctx context.Context) ([]RateRankingProviderSetting, []RateRankingCategoryRule, error) {
	var providers []RateRankingProviderSetting
	if err := r.db.WithContext(ctx).Order("provider ASC").Find(&providers).Error; err != nil {
		return nil, nil, err
	}
	var rules []RateRankingCategoryRule
	if err := r.db.WithContext(ctx).Order("provider ASC, sort_order ASC, id ASC").Find(&rules).Error; err != nil {
		return nil, nil, err
	}
	return providers, rules, nil
}

func (r *RateRankingConfigs) Replace(
	ctx context.Context,
	providers []RateRankingProviderSetting,
	rules []RateRankingCategoryRule,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&RateRankingCategoryRule{}).Error; err != nil {
			return err
		}
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&RateRankingProviderSetting{}).Error; err != nil {
			return err
		}
		if len(providers) > 0 {
			if err := tx.Create(&providers).Error; err != nil {
				return err
			}
		}
		if len(rules) > 0 {
			for i := range rules {
				if err := tx.Create(&rules[i]).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func EncodeRateRankingKeywords(keywords []string) (string, error) {
	data, err := json.Marshal(keywords)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func DecodeRateRankingKeywords(value string) ([]string, error) {
	var keywords []string
	if err := json.Unmarshal([]byte(value), &keywords); err != nil {
		return nil, err
	}
	return keywords, nil
}
