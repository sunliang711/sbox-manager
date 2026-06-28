package traffic

import "time"

const trafficSchemaVersion = 1

// Record 对应 traffic_records 表，保存已落库的周期流量。
type Record struct {
	ID            uint      `gorm:"primaryKey"`
	Instance      string    `gorm:"size:128;not null;uniqueIndex:uidx_traffic_record,priority:1;index:idx_traffic_instance_period_start,priority:1"`
	Server        string    `gorm:"size:255;not null"`
	Period        string    `gorm:"size:16;not null;uniqueIndex:uidx_traffic_record,priority:2;index:idx_traffic_period_start,priority:1;index:idx_traffic_instance_period_start,priority:2"`
	StartTS       int64     `gorm:"column:start_ts;not null;uniqueIndex:uidx_traffic_record,priority:3;index:idx_traffic_period_start,priority:2;index:idx_traffic_instance_period_start,priority:3"`
	EndTS         int64     `gorm:"column:end_ts;not null"`
	StartTime     string    `gorm:"size:32;not null"`
	EndTime       string    `gorm:"size:32;not null"`
	Scope         string    `gorm:"size:32;not null;uniqueIndex:uidx_traffic_record,priority:4;index:idx_traffic_scope_name,priority:1"`
	Name          string    `gorm:"size:255;not null;uniqueIndex:uidx_traffic_record,priority:5;index:idx_traffic_scope_name,priority:2"`
	Direction     string    `gorm:"size:16;not null;uniqueIndex:uidx_traffic_record,priority:6"`
	Bytes         uint64    `gorm:"not null"`
	ResetDetected bool      `gorm:"not null"`
	CreatedAt     time.Time `gorm:"not null"`
}

// TableName 返回 traffic_records 表名。
func (Record) TableName() string {
	return "traffic_records"
}

// Baseline 对应 traffic_baselines 表，保存每个计数的上一轮累计值。
type Baseline struct {
	Instance  string    `gorm:"size:128;primaryKey"`
	Scope     string    `gorm:"size:32;primaryKey"`
	Name      string    `gorm:"size:255;primaryKey"`
	Direction string    `gorm:"size:16;primaryKey"`
	Value     uint64    `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

// TableName 返回 traffic_baselines 表名。
func (Baseline) TableName() string {
	return "traffic_baselines"
}

// Metadata 对应 traffic_metadata 表，保存当前 schema version。
type Metadata struct {
	ID            uint      `gorm:"primaryKey"`
	SchemaVersion int       `gorm:"not null"`
	CreatedAt     time.Time `gorm:"not null"`
	UpdatedAt     time.Time `gorm:"not null"`
}

// TableName 返回 traffic_metadata 表名。
func (Metadata) TableName() string {
	return "traffic_metadata"
}
