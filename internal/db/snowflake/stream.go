package snowflake

import (
	"context"
	"database/sql"
)

type StreamOpts struct{ BatchSize int }

type StreamSink struct {
	Columns func(cols []string)
	Batch   func(rows [][]any, offset int) error
	Done    func(total int64)
	Error   func(err error)
}

func StreamQuery(ctx context.Context, sdb *sql.DB, query string, opts StreamOpts, sink StreamSink) error {
	if opts.BatchSize <= 0 {
		opts.BatchSize = 100
	}
	rows, err := sdb.QueryContext(ctx, query)
	if err != nil {
		reportErr(sink, err)
		return err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		reportErr(sink, err)
		return err
	}
	if sink.Columns != nil {
		sink.Columns(cols)
	}

	batch := make([][]any, 0, opts.BatchSize)
	offset := 0
	var total int64
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			reportErr(sink, err)
			return err
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			reportErr(sink, err)
			return err
		}
		for i, val := range vals {
			if b, ok := val.([]byte); ok {
				vals[i] = string(b)
			}
		}
		batch = append(batch, vals)
		total++
		if len(batch) >= opts.BatchSize {
			if err := flushBatch(ctx, sink, batch, offset); err != nil {
				reportErr(sink, err)
				return err
			}
			offset += len(batch)
			batch = make([][]any, 0, opts.BatchSize)
		}
	}
	if err := rows.Err(); err != nil {
		reportErr(sink, err)
		return err
	}
	if len(batch) > 0 {
		if err := flushBatch(ctx, sink, batch, offset); err != nil {
			reportErr(sink, err)
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		reportErr(sink, err)
		return err
	}
	if sink.Done != nil {
		sink.Done(total)
	}
	return nil
}

func flushBatch(ctx context.Context, sink StreamSink, rows [][]any, offset int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if sink.Batch == nil {
		return nil
	}
	return sink.Batch(rows, offset)
}

func reportErr(sink StreamSink, err error) {
	if sink.Error != nil {
		sink.Error(err)
	}
}
