# SQL Result Limit Design

## Goal

The SQL editor result area should control the maximum number of rows fetched per query. Users choose a row limit in the result area; normal query execution and websocket fallback both obey that limit.

## Scope

- Applies to SQL editor query results shown in `ResultPanel`.
- Applies to `/api/query` and `/ws/query`.
- Does not change table browsing `DataGrid`, which already has `page/per_page`.
- Does not change CSV/SQL export, which intentionally exports full tables.

## Design

Store the selected result limit in the editor store. `ResultPanel` renders a compact selector such as 50, 100, 200, and 500 rows. `SqlEditor` reads that limit and includes it in both the REST query body and websocket exec message.

The backend accepts `max_rows` for `/api/query` and websocket exec messages. It clamps invalid values to the existing default and caps excessively large values. `/api/query` passes the value to `mysql.RunOpts.MaxRows`. Websocket streaming stops after the selected number of rows and marks the final event as truncated.

The result panel remains a display surface for the rows actually fetched. It does not perform client-side pagination over a larger result set, because there is no infinite scroll or server-side cursor design.

## Tests

- Backend test: `/api/query` respects `max_rows`.
- Backend test: websocket streaming stops at `maxRows` and reports truncation.
- Frontend store/component test: selected result limit can be changed.
- Frontend query test: `SqlEditor` sends the selected limit to REST and websocket paths.
