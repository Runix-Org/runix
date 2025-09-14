package db

import (
	"context"
	"errors"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrMustNotBeEmpty          = errors.New("must not be empty")
	ErrMustNotBeGreaterThan255 = errors.New("length must be <= 255")
)

type LaunchCountModel struct {
	ListID      string `gorm:"primaryKey;size:255;not null;index:idx_plugin_list,priority:2"`
	PluginID    string `gorm:"primaryKey;size:255;not null;index:idx_plugin;index:idx_plugin_list,priority:1"`
	ItemID      string `gorm:"primaryKey;size:255;not null"`
	LaunchCount uint   `gorm:"not null;default:0"`
}

func (LaunchCountModel) TableName() string {
	return "launch_counts"
}

type LaunchCountRepo struct {
	db     *gorm.DB
	logger *zap.Logger
}

func (r *LaunchCountRepo) Get(ctx context.Context, listID string, pluginID string) (map[string]uint, bool) {
	const method = "Get"
	if !r.validateListID(listID, method) ||
		!r.validatePluginID(pluginID, method) {
		return nil, false
	}

	var rows []LaunchCountModel
	err := r.db.WithContext(ctx).
		Where("plugin_id = ? AND list_id = ?", pluginID, listID).
		Find(&rows).Error
	if err != nil {
		r.logger.Info("Failed to get launch counts",
			zap.String("method", method),
			zap.String("listID", listID),
			zap.String("pluginID", pluginID),
			zap.Error(err),
		)
		return nil, false
	}

	result := make(map[string]uint, len(rows))
	for _, row := range rows {
		result[row.ItemID] = row.LaunchCount
	}
	return result, true
}

func (r *LaunchCountRepo) Increment(ctx context.Context, listID string, pluginID string, itemID string) (uint, bool) {
	const method = "Increment"
	if !r.validateListID(listID, method) ||
		!r.validatePluginID(pluginID, method) ||
		!r.validateItemID(itemID, method) {
		return 0, false
	}

	row := LaunchCountModel{
		PluginID:    pluginID,
		ListID:      listID,
		ItemID:      itemID,
		LaunchCount: 1,
	}

	// ON CONFLICT (plugin_id, list_id, item_id) DO UPDATE SET launch_count=launch_count+1, updated_at=CURRENT_TIMESTAMP
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "plugin_id"},
				{Name: "list_id"},
				{Name: "item_id"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"launch_count": gorm.Expr("launch_count + 1"),
			}),
		}).
		Create(&row).Error
	if err != nil {
		r.logger.Info("Failed to increment launch count",
			zap.String("method", method),
			zap.String("listID", listID),
			zap.String("pluginID", pluginID),
			zap.String("itemID", itemID),
			zap.Error(err),
		)
		return 0, false
	}

	var result LaunchCountModel
	err = r.db.WithContext(ctx).
		Where("plugin_id = ? AND list_id = ? AND item_id = ?", pluginID, listID, itemID).
		First(&result).Error
	if err != nil {
		r.logger.Info("Failed to get launch count after increment",
			zap.String("method", method),
			zap.String("listID", listID),
			zap.String("pluginID", pluginID),
			zap.String("itemID", itemID),
			zap.Error(err),
		)
		return 0, false
	}

	return result.LaunchCount, true
}

func (r *LaunchCountRepo) DeleteByPlugin(ctx context.Context, pluginID string) (int64, bool) {
	const method = "DeleteByPlugin"
	if !r.validatePluginID(pluginID, method) {
		return 0, false
	}

	res := r.db.WithContext(ctx).
		Where("plugin_id = ?", pluginID).
		Delete(&LaunchCountModel{})
	if res.Error != nil {
		r.logger.Info("Failed to delete items by plugin",
			zap.String("method", method),
			zap.String("pluginID", pluginID),
			zap.Error(res.Error),
		)
		return 0, false
	}

	return res.RowsAffected, true
}

func (r *LaunchCountRepo) DeleteByList(ctx context.Context, listID string) (int64, bool) {
	const method = "DeleteByList"
	if !r.validateListID(listID, method) {
		return 0, false
	}

	res := r.db.WithContext(ctx).
		Where("list_id = ?", listID).
		Delete(&LaunchCountModel{})
	if res.Error != nil {
		r.logger.Info("Failed to delete items by list",
			zap.String("method", method),
			zap.String("listID", listID),
			zap.Error(res.Error),
		)
		return 0, false
	}

	return res.RowsAffected, true
}

func (r *LaunchCountRepo) DeleteItems(
	ctx context.Context,
	listID string,
	pluginID string,
	itemIDs []string,
) (int64, bool) {
	const method = "DeleteItems"
	if !r.validateListID(listID, method) ||
		!r.validatePluginID(pluginID, method) {
		return 0, false
	}
	for _, itemID := range itemIDs {
		if !r.validateItemID(itemID, method) {
			return 0, false
		}
	}
	if len(itemIDs) == 0 {
		return 0, true
	}

	res := r.db.WithContext(ctx).
		Where("plugin_id = ? AND list_id = ? AND item_id IN ?", pluginID, listID, itemIDs).
		Delete(&LaunchCountModel{})
	if res.Error != nil {
		r.logger.Info("Failed to delete items",
			zap.String("method", method),
			zap.String("listID", listID),
			zap.String("pluginID", pluginID),
			zap.Strings("itemIDs", itemIDs),
			zap.Error(res.Error),
		)

		return 0, false
	}

	return res.RowsAffected, true
}

func (r *LaunchCountRepo) validateListID(v string, method string) bool {
	return r.validateField("listID", v, method)
}

func (r *LaunchCountRepo) validatePluginID(v string, method string) bool {
	return r.validateField("pluginID", v, method)
}

func (r *LaunchCountRepo) validateItemID(v string, method string) bool {
	return r.validateField("itemID", v, method)
}

func (r *LaunchCountRepo) validateField(fieldName string, v string, method string) bool {
	var err error
	if v == "" {
		err = ErrMustNotBeEmpty
	} else if len(v) > 255 {
		err = ErrMustNotBeGreaterThan255
	}

	if err != nil {
		r.logger.Warn("Validation failed",
			zap.String("method", method),
			zap.String("field", fieldName),
			zap.String("value", v),
			zap.Error(err),
		)
		return false
	}
	return true
}
