package mssql

import (
	"context"
	"database/sql"
)

// StreamOpts configures streaming behaviour.
type StreamOpts struct {
	BatchSize int
}

// StreamSink receives columnar data as it arrives from the database.
type StreamSink struct {
	Columns func(cols []string)
	Batch   func(rows [][]any, offset int) error
	Done    func(total int64)
	Error   func(err error)
}

// StreamQuery executes query against db and delivers results to sink in
// batches. It is the MSSQL equivalent of the mysql/pg StreamQuery helpers;
// identifiers must use [bracket] quoting and @pN placeholders.
func StreamQuery(ctx context.Context, db *sql.DB, query string, opts StreamOpts, sink StreamSink) error {
	if opts.BatchSize <= 0 {
		opts.BatchSize = 100
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		reportStreamError(sink, err)
		return err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		reportStreamError(sink, err)
		return err
	}
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		reportStreamError(sink, err)
		return err
	}
	dbTypes := columnDatabaseTypes(colTypes)
	if sink.Columns != nil {
		sink.Columns(cols)
	}

	batch := make([][]any, 0, opts.BatchSize)
	offset := 0
	var total int64
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			reportStreamError(sink, err)
			return err
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			reportStreamError(sink, err)
			return err
		}
		for i, val := range vals {
			vals[i] = normalizeValue(val, dbTypes[i])
		}
		batch = append(batch, vals)
		total++
		if len(batch) >= opts.BatchSize {
			if err := emitBatch(ctx, sink, batch, offset); err != nil {
				reportStreamError(sink, err)
				return err
			}
			offset += len(batch)
			batch = make([][]any, 0, opts.BatchSize)
		}
	}
	if err := rows.Err(); err != nil {
		reportStreamError(sink, err)
		return err
	}
	if len(batch) > 0 {
		if err := emitBatch(ctx, sink, batch, offset); err != nil {
			reportStreamError(sink, err)
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		reportStreamError(sink, err)
		return err
	}
	if sink.Done != nil {
		sink.Done(total)
	}
	return nil
}

func emitBatch(ctx context.Context, sink StreamSink, rows [][]any, offset int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if sink.Batch == nil {
		return nil
	}
	return sink.Batch(rows, offset)
}

func reportStreamError(sink StreamSink, err error) {
	if sink.Error != nil {
		sink.Error(err)
	}
}
