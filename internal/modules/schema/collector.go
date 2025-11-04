package schema

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"server_1/internal/core/config"
	"server_1/internal/core/db"
)

const (
	tableLimit    = 1000
	relationLimit = 3000
)

var (
	// ErrDBUnavailable signals that the underlying database cannot be reached.
	ErrDBUnavailable = errors.New("schema: db unavailable")
)

// Collector knows how to extract schema metadata from registered databases.
type Collector struct {
	dbNames []string
}

// NewCollector builds a collector using configured DB names.
func NewCollector() *Collector {
	names := config.DBNames()
	if len(names) == 0 {
		names = []string{"DB1"}
	}
	seen := map[string]struct{}{}
	norm := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		name = strings.ToUpper(name)
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		norm = append(norm, name)
	}
	return &Collector{dbNames: norm}
}

// Collect gathers schema metadata across configured databases.
func (c *Collector) Collect(ctx context.Context) ([]TableDTO, []RelationDTO, bool, bool, error) {
	tables := make([]TableDTO, 0, 64)
	relations := make([]RelationDTO, 0, 64)

	truncatedTables := false
	truncatedRelations := false
	attempted := false

	for _, name := range c.dbNames {
		conn, ok := db.Get(name)
		if !ok || conn == nil || conn.DB == nil {
			continue
		}
		attempted = true

		if err := conn.DB.PingContext(ctx); err != nil {
			return nil, nil, false, false, fmt.Errorf("%w: %v", ErrDBUnavailable, err)
		}

		schemaName, err := currentDatabase(ctx, conn.DB)
		if err != nil {
			return nil, nil, false, false, err
		}

		tableChunk, tableTrunc, err := collectTables(ctx, conn.DB, schemaName)
		if err != nil {
			return nil, nil, false, false, err
		}
		if tableTrunc {
			truncatedTables = true
		}
		tables = append(tables, tableChunk...)
		if len(tables) > tableLimit {
			tables = tables[:tableLimit]
			truncatedTables = true
		}

		relationChunk, relTrunc, err := collectRelations(ctx, conn.DB, schemaName)
		if err != nil {
			return nil, nil, false, false, err
		}
		if relTrunc {
			truncatedRelations = true
		}
		relations = append(relations, relationChunk...)
		if len(relations) > relationLimit {
			relations = relations[:relationLimit]
			truncatedRelations = true
		}

		if len(tables) >= tableLimit && len(relations) >= relationLimit {
			break
		}
	}

	if !attempted {
		return nil, nil, false, false, ErrDBUnavailable
	}

	return tables, relations, truncatedTables, truncatedRelations, nil
}

func currentDatabase(ctx context.Context, sqldb *sql.DB) (string, error) {
	var schemaName sql.NullString
	if err := sqldb.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&schemaName); err != nil {
		return "", wrapDBError(err)
	}
	if !schemaName.Valid || schemaName.String == "" {
		return "", errors.New("schema: unknown database name")
	}
	return schemaName.String, nil
}

func collectTables(ctx context.Context, sqldb *sql.DB, schemaName string) ([]TableDTO, bool, error) {
	const tableQuery = `
SELECT
	TABLE_NAME,
	IFNULL(TABLE_ROWS, 0),
	IFNULL(DATA_LENGTH, 0),
	IFNULL(INDEX_LENGTH, 0)
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = ?
ORDER BY TABLE_NAME
LIMIT ?`

	rows, err := sqldb.QueryContext(ctx, tableQuery, schemaName, tableLimit+1)
	if err != nil {
		return nil, false, wrapDBError(err)
	}
	defer rows.Close()

	tableOrder := make([]string, 0, 64)
	tableMap := make(map[string]*tableMeta)

	for rows.Next() {
		var (
			tableName   string
			rowCount    sql.NullInt64
			dataLength  sql.NullInt64
			indexLength sql.NullInt64
		)
		if err := rows.Scan(&tableName, &rowCount, &dataLength, &indexLength); err != nil {
			return nil, false, wrapDBError(err)
		}

		meta := &tableMeta{
			dto: &TableDTO{
				Schema:      schemaName,
				Name:        tableName,
				Cluster:     inferCluster(tableName),
				RowCount:    nullInt64(rowCount),
				DataSizeMB:  toMB(nullInt64(dataLength)),
				IndexSizeMB: toMB(nullInt64(indexLength)),
			},
		}
		meta.dto.TotalSizeMB = round(meta.dto.DataSizeMB + meta.dto.IndexSizeMB)
		tableMap[tableName] = meta
		tableOrder = append(tableOrder, tableName)
	}
	if err := rows.Err(); err != nil {
		return nil, false, wrapDBError(err)
	}

	truncated := len(tableOrder) > tableLimit
	if truncated {
		tableOrder = tableOrder[:tableLimit]
	}

	if len(tableOrder) == 0 {
		return nil, truncated, nil
	}

	indexAcc := buildIndexMetadata(ctx, sqldb, schemaName, tableMap)
	indexSizes := fetchIndexSizes(ctx, sqldb, schemaName)
	applyIndexSizes(tableMap, indexAcc, indexSizes)

	result := make([]TableDTO, 0, len(tableOrder))
	for _, name := range tableOrder {
		if meta, ok := tableMap[name]; ok && meta.dto != nil {
			result = append(result, *meta.dto)
		}
	}

	return result, truncated, nil
}

func buildIndexMetadata(ctx context.Context, sqldb *sql.DB, schemaName string, tables map[string]*tableMeta) map[string]map[string]*IndexDTO {
	const indexQuery = `
SELECT
	TABLE_NAME,
	INDEX_NAME,
	NON_UNIQUE,
	COLUMN_NAME,
	SEQ_IN_INDEX
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = ?
ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`

	rows, err := sqldb.QueryContext(ctx, indexQuery, schemaName)
	if err != nil {
		return nil
	}
	defer rows.Close()

	indexMap := make(map[string]map[string]*IndexDTO)

	for rows.Next() {
		var (
			tableName string
			indexName string
			nonUnique int
			column    sql.NullString
			seq       sql.NullInt32 // only used to enforce ordering during iteration
		)
		if err := rows.Scan(&tableName, &indexName, &nonUnique, &column, &seq); err != nil {
			return indexMap
		}
		meta, ok := tables[tableName]
		if !ok || meta == nil || meta.dto == nil {
			continue
		}
		tableIndexes, ok := indexMap[tableName]
		if !ok {
			tableIndexes = map[string]*IndexDTO{}
			indexMap[tableName] = tableIndexes
		}
		idx, ok := tableIndexes[indexName]
		if !ok {
			idx = &IndexDTO{
				Name:   indexName,
				Unique: nonUnique == 0,
			}
			tableIndexes[indexName] = idx
		}
		if column.Valid && column.String != "" {
			idx.Columns = append(idx.Columns, column.String)
		}
	}

	if err := rows.Err(); err != nil {
		return indexMap
	}

	return indexMap
}

func fetchIndexSizes(ctx context.Context, sqldb *sql.DB, schemaName string) map[string]map[string]float64 {
	const sizeQuery = `
SELECT
	table_name,
	index_name,
	stat_value
FROM mysql.innodb_index_stats
WHERE database_name = ?
	AND stat_name = 'size'`

	rows, err := sqldb.QueryContext(ctx, sizeQuery, schemaName)
	if err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "doesn't exist") ||
			strings.Contains(lower, "unknown table") ||
			strings.Contains(lower, "access denied") {
			return nil
		}
		return nil
	}
	defer rows.Close()

	sizes := make(map[string]map[string]float64)
	for rows.Next() {
		var (
			tableName string
			indexName string
			statValue string
		)
		if err := rows.Scan(&tableName, &indexName, &statValue); err != nil {
			return sizes
		}
		pages, err := strconv.ParseFloat(statValue, 64)
		if err != nil {
			continue
		}
		sizeMB := (pages * 16.0) / 1024.0 // each page is 16KB
		if sizes[tableName] == nil {
			sizes[tableName] = make(map[string]float64)
		}
		sizes[tableName][indexName] = round(sizeMB)
	}
	if err := rows.Err(); err != nil {
		return sizes
	}
	return sizes
}

func applyIndexSizes(tables map[string]*tableMeta, indexMap map[string]map[string]*IndexDTO, indexSizes map[string]map[string]float64) {
	if len(indexMap) == 0 {
		return
	}

	for tableName, indexes := range indexMap {
		meta, ok := tables[tableName]
		if !ok || meta == nil || meta.dto == nil {
			continue
		}

		count := len(indexes)
		share := 0.0
		if count > 0 && meta.dto.IndexSizeMB > 0 {
			share = meta.dto.IndexSizeMB / float64(count)
		}

		meta.dto.Indexes = make([]IndexDTO, 0, count)
		indexNames := make([]string, 0, len(indexes))
		for name := range indexes {
			indexNames = append(indexNames, name)
		}
		sort.Strings(indexNames)

		for _, name := range indexNames {
			idx := indexes[name]
			item := *idx
			if len(idx.Columns) > 0 {
				cols := make([]string, len(idx.Columns))
				copy(cols, idx.Columns)
				item.Columns = cols
			}
			size := share
			if tableSizes, ok := indexSizes[tableName]; ok {
				if val, ok := tableSizes[name]; ok {
					size = val
				}
			}
			item.SizeMB = round(size)
			meta.dto.Indexes = append(meta.dto.Indexes, item)
		}
	}
}

func collectRelations(ctx context.Context, sqldb *sql.DB, schemaName string) ([]RelationDTO, bool, error) {
	const relationQuery = `
SELECT
	CONSTRAINT_NAME,
	TABLE_NAME,
	COLUMN_NAME,
	REFERENCED_TABLE_SCHEMA,
	REFERENCED_TABLE_NAME,
	REFERENCED_COLUMN_NAME
FROM information_schema.KEY_COLUMN_USAGE
WHERE TABLE_SCHEMA = ?
	AND REFERENCED_TABLE_NAME IS NOT NULL
ORDER BY TABLE_NAME, COLUMN_NAME
LIMIT ?`

	rows, err := sqldb.QueryContext(ctx, relationQuery, schemaName, relationLimit+1)
	if err != nil {
		return nil, false, wrapDBError(err)
	}
	defer rows.Close()

	relations := make([]RelationDTO, 0, 32)
	for rows.Next() {
		var (
			constraintName string
			tableName      string
			columnName     string
			refSchema      sql.NullString
			refTable       sql.NullString
			refColumn      sql.NullString
		)
		if err := rows.Scan(&constraintName, &tableName, &columnName, &refSchema, &refTable, &refColumn); err != nil {
			return nil, false, wrapDBError(err)
		}
		relations = append(relations, RelationDTO{
			Schema:           schemaName,
			Constraint:       constraintName,
			FromTable:        tableName,
			FromColumn:       columnName,
			ReferencedSchema: nullString(refSchema, schemaName),
			ToTable:          refTable.String,
			ToColumn:         refColumn.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, false, wrapDBError(err)
	}

	truncated := len(relations) > relationLimit
	if truncated {
		relations = relations[:relationLimit]
	}

	return relations, truncated, nil
}

func wrapDBError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: %v", ErrDBUnavailable, err)
	}
	if errors.Is(err, sql.ErrConnDone) || errors.Is(err, driver.ErrBadConn) {
		return fmt.Errorf("%w: %v", ErrDBUnavailable, err)
	}
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%w: %v", ErrDBUnavailable, err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "connection refused") ||
		strings.Contains(strings.ToLower(err.Error()), "no such host") {
		return fmt.Errorf("%w: %v", ErrDBUnavailable, err)
	}
	return err
}

func inferCluster(tableName string) string {
	idx := strings.Index(tableName, "_")
	if idx > 0 {
		return tableName[:idx]
	}
	return "core"
}

func nullInt64(v sql.NullInt64) int64 {
	if v.Valid {
		return v.Int64
	}
	return 0
}

func nullString(v sql.NullString, fallback string) string {
	if v.Valid && v.String != "" {
		return v.String
	}
	return fallback
}

func toMB(bytes int64) float64 {
	if bytes <= 0 {
		return 0
	}
	return round(float64(bytes) / (1024.0 * 1024.0))
}

func round(val float64) float64 {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0
	}
	return math.Round(val*100) / 100
}

// tableMeta is defined after the collector functions to limit exported surface.
type tableMeta struct {
	dto *TableDTO
}
