package db

import "database/sql"

// querier 是 *sql.DB 和 *sql.Tx 共有的查询入口。
type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// queryAll 执行查询，用 scan 把每一行读进一个新的 T，并按顺序收集成切片。
//
// 没有任何行时返回 nil 切片（序列化成 JSON 是 null），与各查询函数原先手写循环的
// 行为一致。scan 每次拿到的都是零值的 T，不会带着上一行的内容。
func queryAll[T any](q querier, scan func(rows *sql.Rows, dst *T) error, query string, args ...any) ([]T, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []T
	for rows.Next() {
		var item T
		if err = scan(rows, &item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
