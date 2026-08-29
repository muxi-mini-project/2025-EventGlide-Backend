// PII 存量数据加密迁移工具（一次性 CLI）。
// 将历史明文真名/姓名快照加密为 v1: 密文；已有 v1: 前缀的行跳过（幂等，可重跑）。
// 用法：
//   export EG_PII_KEY="<与 Nacos piiKey 一致>"
//   go run ./tools/migrate_pii -dsn "user:pass@tcp(host:3306)/db?charset=utf8mb4&parseTime=true" -dry-run   # 预览
//   go run ./tools/migrate_pii -dsn "user:pass@tcp(host:3306)/db?charset=utf8mb4&parseTime=true"          # 执行
// key 从环境变量 EG_PII_KEY 读取（避免出现在进程参数中），需与运行时的 Nacos piiKey 一致。
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/raiki02/EG/pkg/encrypt"
)

type target struct {
	table   string
	idCol   string
	columns []string
}

var targets = []target{
	{table: "user", idCol: "id", columns: []string{"real_name"}},
	{table: "activity_signer", idCol: "id", columns: []string{"name"}},
	{table: "approvement", idCol: "id", columns: []string{"student_name"}},
	{table: "comment", idCol: "id", columns: []string{"creator_name", "reply_to_user_name"}},
}

const batchSize = 500

type change struct {
	col string
	enc string
}

func main() {
	dsn := flag.String("dsn", "", "MySQL DSN")
	dryRun := flag.Bool("dry-run", false, "只统计不更新")
	flag.Parse()

	if *dsn == "" {
		log.Fatal("需提供 -dsn")
	}
	// key 仅从环境变量 EG_PII_KEY 读取（避免密钥出现在进程参数/ps 中），需与运行时 Nacos piiKey 一致
	if err := encrypt.ValidateKey(); err != nil {
		log.Fatalf("PII 密钥校验失败: %v", err)
	}

	db, err := sql.Open("mysql", *dsn)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)

	failed := false
	for _, t := range targets {
		if err := migrateTable(db, t, *dryRun); err != nil {
			failed = true
			log.Printf("[%s] 迁移失败: %v", t.table, err)
		}
	}
	if failed {
		os.Exit(1)
	}
}

func migrateTable(db *sql.DB, t target, dryRun bool) error {
	var lastID int64
	var processed, skipped, failed int
	for {
		rows, err := loadBatch(db, t, lastID, batchSize)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			changes := make([]change, 0, len(t.columns))
			for _, col := range t.columns {
				raw := r.values[col]
				if raw == "" || strings.HasPrefix(raw, encrypt.Prefix) {
					continue
				}
				enc, err := encrypt.Encrypt(raw)
				if err != nil {
					log.Printf("[%s] id=%d %s 加密失败: %v", t.table, r.id, col, err)
					failed++
					continue
				}
				changes = append(changes, change{col: col, enc: enc})
			}
			if len(changes) == 0 {
				skipped++
				continue
			}
			if dryRun {
				for _, c := range changes {
					log.Printf("[dry-run] %s id=%d %s: 将加密（不输出明文值）", t.table, r.id, c.col)
				}
			} else if err := updateRow(db, t, r.id, changes); err != nil {
				log.Printf("[%s] id=%d 更新失败: %v", t.table, r.id, err)
				failed++
				continue
			}
			processed++
		}
		lastID = rows[len(rows)-1].id
		log.Printf("[%s] 进度 %d（跳过 %d，失败 %d）", t.table, processed, skipped, failed)
	}
	log.Printf("[%s] 完成：处理 %d 行，跳过 %d，失败 %d", t.table, processed, skipped, failed)
	return nil
}

type dbRow struct {
	id     int64
	values map[string]string
}

func loadBatch(db *sql.DB, t target, afterID int64, limit int) ([]dbRow, error) {
	var conds []string
	for _, col := range t.columns {
		conds = append(conds, fmt.Sprintf("%s NOT LIKE 'v1:%%'", col))
	}
	cols := strings.Join(t.columns, ", ")
	q := fmt.Sprintf("SELECT %s, %s FROM %s WHERE %s > ? AND (%s) ORDER BY %s LIMIT ?",
		t.idCol, cols, t.table, t.idCol, strings.Join(conds, " OR "), t.idCol)

	rows, err := db.Query(q, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []dbRow
	for rows.Next() {
		dest := make([]interface{}, 1, len(t.columns)+1)
		var id int64
		dest[0] = &id
		colVals := make([]sql.NullString, len(t.columns))
		for i := range colVals {
			dest = append(dest, &colVals[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		r := dbRow{id: id, values: make(map[string]string, len(t.columns))}
		for i, col := range t.columns {
			if colVals[i].Valid {
				r.values[col] = colVals[i].String
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func updateRow(db *sql.DB, t target, id int64, changes []change) error {
	sets := make([]string, len(changes))
	args := make([]interface{}, 0, len(changes)+1)
	for i, c := range changes {
		sets[i] = fmt.Sprintf("%s = ?", c.col)
		args = append(args, c.enc)
	}
	args = append(args, id)
	q := fmt.Sprintf("UPDATE %s SET %s WHERE %s = ?", t.table, strings.Join(sets, ", "), t.idCol)
	_, err := db.Exec(q, args...)
	return err
}
