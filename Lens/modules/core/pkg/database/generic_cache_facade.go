package database

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GenericCacheFacadeInterface defines the database operation interface for GenericCache
type GenericCacheFacadeInterface interface {
	// Get retrieves a cache entry by key and unmarshals it into the provided value
	Get(ctx context.Context, key string, value interface{}) error
	// Set stores a cache entry with the given key and value, with optional expiration
	Set(ctx context.Context, key string, value interface{}, expiresAt *time.Time) error
	// Delete removes a cache entry by key
	Delete(ctx context.Context, key string) error
	// DeleteExpired removes all expired cache entries
	DeleteExpired(ctx context.Context) error
	// WithCluster method
	WithCluster(clusterName string) GenericCacheFacadeInterface
}

// GenericCacheFacade implements GenericCacheFacadeInterface
type GenericCacheFacade struct {
	BaseFacade
}

// NewGenericCacheFacade creates a new GenericCacheFacade instance
func NewGenericCacheFacade() GenericCacheFacadeInterface {
	return &GenericCacheFacade{}
}

func (f *GenericCacheFacade) WithCluster(clusterName string) GenericCacheFacadeInterface {
	return &GenericCacheFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Get retrieves a cache entry by key and unmarshals it into the provided value
func (f *GenericCacheFacade) Get(ctx context.Context, key string, value interface{}) error {
	q := f.getDAL().GenericCache
	cache, err := q.WithContext(ctx).Where(q.Key.Eq(key)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Debugf("GenericCache Get: no record found for key: %s", key)
			return gorm.ErrRecordNotFound
		}
		log.Errorf("GenericCache Get: error querying key %s: %v", key, err)
		return err
	}

	// Check if expired
	if !cache.ExpiresAt.IsZero() && cache.ExpiresAt.Before(time.Now()) {
		log.Debugf("GenericCache Get: cache entry for key %s has expired", key)
		return gorm.ErrRecordNotFound
	}

	// Unmarshal the value
	valueBytes, err := json.Marshal(cache.Value)
	if err != nil {
		log.Errorf("GenericCache Get: error marshaling cache value: %v", err)
		return err
	}

	// Check if it's a wrapped array with BUILTIN_ARRAY field
	var tempMap map[string]interface{}
	if err := json.Unmarshal(valueBytes, &tempMap); err == nil {
		if arrayValue, exists := tempMap["BUILTIN_ARRAY"]; exists {
			// Extract the array from BUILTIN_ARRAY field
			arrayBytes, err := json.Marshal(arrayValue)
			if err != nil {
				log.Errorf("GenericCache Get: error marshaling BUILTIN_ARRAY value: %v", err)
				return err
			}
			err = json.Unmarshal(arrayBytes, value)
			if err != nil {
				log.Errorf("GenericCache Get: error unmarshaling BUILTIN_ARRAY value: %v", err)
				return err
			}
			return nil
		}
	}

	// Normal unmarshal for non-array values
	err = json.Unmarshal(valueBytes, value)
	if err != nil {
		log.Errorf("GenericCache Get: error unmarshaling cache value: %v", err)
		return err
	}

	return nil
}

// Set stores a cache entry with the given key and value, with optional expiration
func (f *GenericCacheFacade) Set(ctx context.Context, key string, value interface{}, expiresAt *time.Time) error {
	// Marshal the value to ExtType
	valueBytes, err := json.Marshal(value)
	if err != nil {
		log.Errorf("GenericCache Set: error marshaling value: %v", err)
		return err
	}

	// Check if the value is a JSON array
	var rawValue interface{}
	err = json.Unmarshal(valueBytes, &rawValue)
	if err != nil {
		log.Errorf("GenericCache Set: error unmarshaling value: %v", err)
		return err
	}

	// If it's an array, wrap it in BUILTIN_ARRAY field
	if _, isArray := rawValue.([]interface{}); isArray {
		wrappedValue := map[string]interface{}{
			"BUILTIN_ARRAY": rawValue,
		}
		valueBytes, err = json.Marshal(wrappedValue)
		if err != nil {
			log.Errorf("GenericCache Set: error marshaling wrapped array: %v", err)
			return err
		}
	}

	var extValue model.ExtType
	err = json.Unmarshal(valueBytes, &extValue)
	if err != nil {
		log.Errorf("GenericCache Set: error converting to ExtType: %v", err)
		return err
	}

	cache := &model.GenericCache{
		Key:   key,
		Value: extValue,
	}

	if expiresAt != nil {
		cache.ExpiresAt = *expiresAt
	}

	// Use upsert logic: insert or update
	db := f.getDB()
	err = db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "expires_at", "updated_at"}),
	}).Create(cache).Error

	if err != nil {
		log.Errorf("GenericCache Set: error upserting cache entry: %v", err)
		return err
	}

	return nil
}

// Delete removes a cache entry by key
func (f *GenericCacheFacade) Delete(ctx context.Context, key string) error {
	q := f.getDAL().GenericCache
	result, err := q.WithContext(ctx).Where(q.Key.Eq(key)).Delete()
	if err != nil {
		log.Errorf("GenericCache Delete: error deleting key %s: %v", key, err)
		return err
	}

	if result.RowsAffected == 0 {
		log.Debugf("GenericCache Delete: no record found for key: %s", key)
	}

	return nil
}

// DeleteExpired removes all expired cache entries
func (f *GenericCacheFacade) DeleteExpired(ctx context.Context) error {
	q := f.getDAL().GenericCache
	now := time.Now()
	result, err := q.WithContext(ctx).
		Where(q.ExpiresAt.IsNotNull()).
		Where(q.ExpiresAt.Lt(now)).
		Delete()

	if err != nil {
		log.Errorf("GenericCache DeleteExpired: error deleting expired entries: %v", err)
		return err
	}

	if result.RowsAffected > 0 {
		log.Infof("GenericCache DeleteExpired: deleted %d expired entries", result.RowsAffected)
	}

	return nil
}
