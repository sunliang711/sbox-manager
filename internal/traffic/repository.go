package traffic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const sqliteBusyTimeoutMS = 5000

// Repository 隔离 traffic SQLite 的 GORM 访问细节。
type Repository struct {
	db       *gorm.DB
	readOnly bool
}

// OpenRepository 打开并迁移 traffic SQLite 数据库。
func OpenRepository(path string) (*Repository, error) {
	return openRepository(path, false)
}

// OpenRepositoryReadOnly 打开只读查询用 traffic SQLite 数据库，不迁移、不写 metadata。
func OpenRepositoryReadOnly(path string) (*Repository, error) {
	return openRepository(path, true)
}

func openRepository(path string, readOnly bool) (*Repository, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("traffic db path cannot be empty")
	}
	if !isSpecialSQLiteDSN(path) {
		if readOnly {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				return openRepository(":memory:", true)
			} else if err != nil {
				return nil, fmt.Errorf("read traffic DB %s: %w", path, err)
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			return nil, fmt.Errorf("create traffic DB directory: %w", err)
		}
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open traffic DB: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get traffic DB connection: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)
	if !readOnly {
		if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
			return nil, fmt.Errorf("enable SQLite WAL: %w", err)
		}
	}
	if err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d", sqliteBusyTimeoutMS)).Error; err != nil {
		return nil, fmt.Errorf("set SQLite busy_timeout: %w", err)
	}
	if err := validateExistingSchemaVersion(db); err != nil {
		return nil, err
	}
	if readOnly {
		return &Repository{db: db, readOnly: true}, nil
	}
	if err := db.AutoMigrate(&Record{}, &Baseline{}, &Metadata{}); err != nil {
		return nil, fmt.Errorf("migrate traffic DB: %w", err)
	}
	repo := &Repository{db: db}
	if err := repo.ensureMetadata(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

// Close 关闭底层 SQLite 连接。
func (r *Repository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// GetBaseline 查询单个累计计数的 baseline。
func (r *Repository) GetBaseline(ctx context.Context, instance string, counter Counter) (uint64, bool, error) {
	var baseline Baseline
	err := r.db.WithContext(ctx).First(&baseline, "instance = ? AND scope = ? AND name = ? AND direction = ?", instance, counter.Scope, counter.Name, counter.Direction).Error
	if err == nil {
		return baseline.Value, true, nil
	}
	if err == gorm.ErrRecordNotFound {
		return 0, false, nil
	}
	if r.readOnly && isMissingTrafficTableError(err) {
		return 0, false, nil
	}
	return 0, false, fmt.Errorf("query traffic baseline: %w", err)
}

// UpsertBaseline 写入或更新单个累计计数的 baseline。
func (r *Repository) UpsertBaseline(ctx context.Context, instance string, counter Counter, now time.Time) error {
	baseline := Baseline{
		Instance:  instance,
		Scope:     counter.Scope,
		Name:      counter.Name,
		Direction: counter.Direction,
		Value:     counter.Value,
		UpdatedAt: now,
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "instance"},
			{Name: "scope"},
			{Name: "name"},
			{Name: "direction"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&baseline).Error
	if err != nil {
		return fmt.Errorf("update traffic baseline: %w", err)
	}
	return nil
}

// AddHourlyCounters 在同一事务中写入 hourly delta 并更新 baseline。
func (r *Repository) AddHourlyCounters(ctx context.Context, target Target, counters []Counter, window TimeRange, now time.Time, location *time.Location) ([]Record, error) {
	records := make([]Record, 0, len(counters))
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, counter := range counters {
			baseline, exists, err := getBaselineTx(tx, target.Instance, counter)
			if err != nil {
				return err
			}
			delta := counter.Value
			resetDetected := false
			if exists {
				if counter.Value >= baseline {
					delta = counter.Value - baseline
				} else {
					resetDetected = true
				}
			}
			if delta > 0 {
				record := recordFromCounter(target, PeriodHourly, window, counter, delta, resetDetected, now, location)
				if err := addRecordTx(tx, record); err != nil {
					return err
				}
				records = append(records, record)
			}
			if err := upsertBaselineTx(tx, target.Instance, counter, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return stableRecordOrder(records), nil
}

// AddRecords 将增量记录累加写入 traffic_records。
func (r *Repository) AddRecords(ctx context.Context, records []Record) error {
	for _, record := range records {
		if record.Bytes == 0 {
			continue
		}
		if err := addRecordTx(r.db.WithContext(ctx), record); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceRecords 用聚合结果覆盖写入 traffic_records。
func (r *Repository) ReplaceRecords(ctx context.Context, records []Record) error {
	for _, record := range records {
		err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: recordConflictColumns(),
			DoUpdates: clause.AssignmentColumns([]string{
				"server",
				"end_ts",
				"end_time",
				"bytes",
				"reset_detected",
			}),
		}).Create(&record).Error
		if err != nil {
			return fmt.Errorf("write traffic aggregate: %w", err)
		}
	}
	return nil
}

// ReplaceWindowRecords 删除指定周期窗口的旧聚合后写入新记录。
func (r *Repository) ReplaceWindowRecords(ctx context.Context, period string, instances []string, window TimeRange, records []Record) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Where("period = ? AND start_ts = ?", period, window.Start.Unix())
		if len(instances) > 0 {
			query = query.Where("instance IN ?", instances)
		}
		if err := query.Delete(&Record{}).Error; err != nil {
			return fmt.Errorf("delete old traffic aggregate: %w", err)
		}
		for _, record := range records {
			if err := tx.Create(&record).Error; err != nil {
				return fmt.Errorf("write traffic aggregate: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// ListRecords 查询指定条件下的 traffic_records。
func (r *Repository) ListRecords(ctx context.Context, period string, filter Filter, timeRange TimeRange) ([]Record, error) {
	query := r.db.WithContext(ctx).Model(&Record{}).Where("period = ?", period)
	if !timeRange.Start.IsZero() {
		query = query.Where("end_ts > ?", timeRange.Start.Unix())
	}
	if !timeRange.End.IsZero() {
		query = query.Where("start_ts < ?", timeRange.End.Unix())
	}
	if len(filter.Instances) > 0 {
		query = query.Where("instance IN ?", filter.Instances)
	}
	if filter.Scope != "" {
		query = query.Where("scope = ?", filter.Scope)
	}
	if filter.Name != "" {
		query = query.Where("name = ?", filter.Name)
	}
	query = query.Order("start_ts ASC").Order("instance ASC").Order("scope ASC").Order("name ASC").Order("direction ASC")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	var records []Record
	if err := query.Find(&records).Error; err != nil {
		if r.readOnly && isMissingTrafficTableError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("query traffic records: %w", err)
	}
	return records, nil
}

// CountBefore 统计指定周期中早于 cutoff 的记录数。
func (r *Repository) CountBefore(ctx context.Context, period string, cutoff time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Record{}).Where("period = ? AND start_ts < ?", period, cutoff.Unix()).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count traffic records to clean: %w", err)
	}
	return count, nil
}

// DeleteBefore 删除指定周期中早于 cutoff 的记录。
func (r *Repository) DeleteBefore(ctx context.Context, period string, cutoff time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Where("period = ? AND start_ts < ?", period, cutoff.Unix()).Delete(&Record{})
	if result.Error != nil {
		return 0, fmt.Errorf("clean traffic records: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// ensureMetadata 确保 traffic_metadata 中存在 schema version。
func (r *Repository) ensureMetadata(ctx context.Context) error {
	now := time.Now().UTC()
	var existing Metadata
	err := r.db.WithContext(ctx).First(&existing, "id = ?", 1).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("read traffic metadata: %w", err)
	}
	if err == nil && existing.SchemaVersion > trafficSchemaVersion {
		return fmt.Errorf("traffic DB schema version %d is higher than supported version %d", existing.SchemaVersion, trafficSchemaVersion)
	}
	metadata := Metadata{
		ID:            1,
		SchemaVersion: trafficSchemaVersion,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err == nil {
		metadata.CreatedAt = existing.CreatedAt
	}
	err = r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"schema_version", "updated_at"}),
	}).Create(&metadata).Error
	if err != nil {
		return fmt.Errorf("write traffic metadata: %w", err)
	}
	return nil
}

func validateExistingSchemaVersion(db *gorm.DB) error {
	var tableName string
	if err := db.Raw("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", "traffic_metadata").Scan(&tableName).Error; err != nil {
		return fmt.Errorf("check traffic metadata table: %w", err)
	}
	if tableName == "" {
		return nil
	}
	var metadata Metadata
	err := db.Table("traffic_metadata").Select("schema_version").Where("id = ?", 1).Take(&metadata).Error
	if err == gorm.ErrRecordNotFound {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read traffic metadata: %w", err)
	}
	if metadata.SchemaVersion > trafficSchemaVersion {
		return fmt.Errorf("traffic DB schema version %d is higher than supported version %d", metadata.SchemaVersion, trafficSchemaVersion)
	}
	return nil
}

func getBaselineTx(tx *gorm.DB, instance string, counter Counter) (uint64, bool, error) {
	var baseline Baseline
	err := tx.First(&baseline, "instance = ? AND scope = ? AND name = ? AND direction = ?", instance, counter.Scope, counter.Name, counter.Direction).Error
	if err == nil {
		return baseline.Value, true, nil
	}
	if err == gorm.ErrRecordNotFound {
		return 0, false, nil
	}
	return 0, false, fmt.Errorf("query traffic baseline: %w", err)
}

func upsertBaselineTx(tx *gorm.DB, instance string, counter Counter, now time.Time) error {
	baseline := Baseline{
		Instance:  instance,
		Scope:     counter.Scope,
		Name:      counter.Name,
		Direction: counter.Direction,
		Value:     counter.Value,
		UpdatedAt: now,
	}
	err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "instance"},
			{Name: "scope"},
			{Name: "name"},
			{Name: "direction"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&baseline).Error
	if err != nil {
		return fmt.Errorf("update traffic baseline: %w", err)
	}
	return nil
}

func addRecordTx(tx *gorm.DB, record Record) error {
	err := tx.Clauses(clause.OnConflict{
		Columns: recordConflictColumns(),
		DoUpdates: clause.Assignments(map[string]interface{}{
			"server":         record.Server,
			"end_ts":         record.EndTS,
			"end_time":       record.EndTime,
			"bytes":          gorm.Expr("traffic_records.bytes + ?", record.Bytes),
			"reset_detected": gorm.Expr("traffic_records.reset_detected OR ?", record.ResetDetected),
		}),
	}).Create(&record).Error
	if err != nil {
		return fmt.Errorf("write traffic record: %w", err)
	}
	return nil
}

// recordConflictColumns 返回 traffic_records 的唯一键列。
func recordConflictColumns() []clause.Column {
	return []clause.Column{
		{Name: "instance"},
		{Name: "period"},
		{Name: "start_ts"},
		{Name: "scope"},
		{Name: "name"},
		{Name: "direction"},
	}
}

// isSpecialSQLiteDSN 判断是否是无需创建父目录的 SQLite DSN。
func isSpecialSQLiteDSN(path string) bool {
	return path == ":memory:" || strings.HasPrefix(path, "file:")
}

func isMissingTrafficTableError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "no such table: traffic_records") ||
		strings.Contains(text, "no such table: traffic_baselines") ||
		strings.Contains(text, "no such table: traffic_metadata")
}
