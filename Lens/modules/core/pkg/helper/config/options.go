package config

import "gorm.io/gorm"

// setOptions defines optional parameters for Set operations
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

// SetOption is a function option for configuring Set operations
type SetOption func(*setOptions)

// WithDescription sets the configuration description
func WithDescription(description string) SetOption {
	return func(o *setOptions) {
		o.description = description
	}
}

// WithCategory sets the configuration category
func WithCategory(category string) SetOption {
	return func(o *setOptions) {
		o.category = category
	}
}

// WithEncrypted marks configuration as encrypted
func WithEncrypted(encrypted bool) SetOption {
	return func(o *setOptions) {
		o.isEncrypted = encrypted
	}
}

// WithReadonly marks configuration as readonly
func WithReadonly(readonly bool) SetOption {
	return func(o *setOptions) {
		o.isReadonly = readonly
	}
}

// WithCreatedBy sets the creator
func WithCreatedBy(createdBy string) SetOption {
	return func(o *setOptions) {
		o.createdBy = createdBy
	}
}

// WithUpdatedBy sets the updater
func WithUpdatedBy(updatedBy string) SetOption {
	return func(o *setOptions) {
		o.updatedBy = updatedBy
	}
}

// WithRecordHistory sets whether to record history
func WithRecordHistory(record bool) SetOption {
	return func(o *setOptions) {
		o.recordHistory = record
	}
}

// WithChangeReason sets the reason for change
func WithChangeReason(reason string) SetOption {
	return func(o *setOptions) {
		o.changeReason = reason
	}
}

// ListFilter is a filter function for list queries
type ListFilter func(*gorm.DB) *gorm.DB

// WithCategoryFilter filters by category
func WithCategoryFilter(category string) ListFilter {
	return func(db *gorm.DB) *gorm.DB {
		if category != "" {
			return db.Where("category = ?", category)
		}
		return db
	}
}

// WithReadonlyFilter filters by readonly status
func WithReadonlyFilter(readonly bool) ListFilter {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("is_readonly = ?", readonly)
	}
}

// WithEncryptedFilter filters by encryption status
func WithEncryptedFilter(encrypted bool) ListFilter {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("is_encrypted = ?", encrypted)
	}
}

// WithOrderBy sets the ordering
func WithOrderBy(orderBy string) ListFilter {
	return func(db *gorm.DB) *gorm.DB {
		if orderBy != "" {
			return db.Order(orderBy)
		}
		return db
	}
}

// WithLimit limits the number of results
func WithLimit(limit int) ListFilter {
	return func(db *gorm.DB) *gorm.DB {
		if limit > 0 {
			return db.Limit(limit)
		}
		return db
	}
}
