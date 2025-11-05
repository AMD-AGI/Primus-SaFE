package config

import "gorm.io/gorm"

// setOptions 定义 Set 操作的可选参数
type setOptions struct {
	description   string
	category      string
	isEncrypted   bool
	isReadonly    bool
	createdBy     string
	updatedBy     string
	recordHistory bool
	changeReason  string
}

// SetOption 是配置 Set 操作的函数选项
type SetOption func(*setOptions)

// WithDescription 设置配置描述
func WithDescription(description string) SetOption {
	return func(o *setOptions) {
		o.description = description
	}
}

// WithCategory 设置配置类别
func WithCategory(category string) SetOption {
	return func(o *setOptions) {
		o.category = category
	}
}

// WithEncrypted 标记配置为加密
func WithEncrypted(encrypted bool) SetOption {
	return func(o *setOptions) {
		o.isEncrypted = encrypted
	}
}

// WithReadonly 标记配置为只读
func WithReadonly(readonly bool) SetOption {
	return func(o *setOptions) {
		o.isReadonly = readonly
	}
}

// WithCreatedBy 设置创建者
func WithCreatedBy(createdBy string) SetOption {
	return func(o *setOptions) {
		o.createdBy = createdBy
	}
}

// WithUpdatedBy 设置更新者
func WithUpdatedBy(updatedBy string) SetOption {
	return func(o *setOptions) {
		o.updatedBy = updatedBy
	}
}

// WithRecordHistory 设置是否记录历史
func WithRecordHistory(record bool) SetOption {
	return func(o *setOptions) {
		o.recordHistory = record
	}
}

// WithChangeReason 设置变更原因
func WithChangeReason(reason string) SetOption {
	return func(o *setOptions) {
		o.changeReason = reason
	}
}

// ListFilter 是列表查询的过滤器函数
type ListFilter func(*gorm.DB) *gorm.DB

// WithCategoryFilter 根据类别过滤
func WithCategoryFilter(category string) ListFilter {
	return func(db *gorm.DB) *gorm.DB {
		if category != "" {
			return db.Where("category = ?", category)
		}
		return db
	}
}

// WithReadonlyFilter 根据只读状态过滤
func WithReadonlyFilter(readonly bool) ListFilter {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("is_readonly = ?", readonly)
	}
}

// WithEncryptedFilter 根据加密状态过滤
func WithEncryptedFilter(encrypted bool) ListFilter {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("is_encrypted = ?", encrypted)
	}
}

// WithOrderBy 设置排序
func WithOrderBy(orderBy string) ListFilter {
	return func(db *gorm.DB) *gorm.DB {
		if orderBy != "" {
			return db.Order(orderBy)
		}
		return db
	}
}

// WithLimit 限制返回数量
func WithLimit(limit int) ListFilter {
	return func(db *gorm.DB) *gorm.DB {
		if limit > 0 {
			return db.Limit(limit)
		}
		return db
	}
}
