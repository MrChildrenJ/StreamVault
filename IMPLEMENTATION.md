# Implementation Notes

## Repository Pattern

**職責**：封裝所有 SQL 操作，讓 Service 只管商業邏輯，不碰 DB 細節。

```
Handler（HTTP）
  └── Service（商業邏輯）
        └── Repository（SQL）
              └── pgxpool.Pool（連線池）→ PostgreSQL
```

**pgxpool.Pool**：維護一組已建好的 DB 連線，request 進來借一條，用完歸還，避免每次重新建連線的開銷。

**為什麼 Repository method 收 `pgx.Tx` 而非直接用 `r.db`**：
多個 Repository 操作需要在同一個 DB transaction 裡執行（例如 TopUp = credit wallet + insert transaction）。
傳入 `pgx.Tx` 讓 Service 控制 transaction 的邊界，Repository 只負責執行 SQL。

```go
// Service 控制 tx 生命週期
withTx(ctx, s.db, func(tx pgx.Tx) error {
    s.wallets.Credit(ctx, tx, ...)   // 同一個 tx
    s.txs.Create(ctx, tx, ...)       // 同一個 tx → 失敗則一起 rollback
})
```
