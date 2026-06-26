package app

import (
	"context"
	"fmt"

	databaseinfra "proxygateway/internal/infrastructure/database"
	sqliteinfra "proxygateway/internal/infrastructure/sqlite"
)

func (g *Gateway) migrate() error {
	switch g.dbDialect {
	case "", databaseinfra.DialectSQLite:
		if err := sqliteinfra.Migrate(context.Background(), g.db); err != nil {
			return err
		}
	case databaseinfra.DialectPostgres:
		return fmt.Errorf("postgres migrations are not implemented yet")
	default:
		return fmt.Errorf("unsupported database dialect %q", g.dbDialect)
	}
	if err := g.backfillNodeOutboundJSON(); err != nil {
		return err
	}
	return nil
}

func (g *Gateway) backfillNodeOutboundJSON() error {
	rows, err := g.db.Query(`SELECT id, name, type, server, server_port, username, password, outbound_json FROM nodes ORDER BY created_at, id`)
	if err != nil {
		return err
	}
	type row struct {
		id           string
		node         parsedSubscriptionNode
		outboundJSON string
	}
	var nodes []row
	for rows.Next() {
		var item row
		if err := rows.Scan(
			&item.id,
			&item.node.Name,
			&item.node.Type,
			&item.node.Server,
			&item.node.ServerPort,
			&item.node.Username,
			&item.node.Password,
			&item.outboundJSON,
		); err != nil {
			_ = rows.Close()
			return err
		}
		item.node.OutboundJSON = item.outboundJSON
		nodes = append(nodes, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range nodes {
		outboundJSON, err := normalizedNodeOutboundJSON(item.node)
		if err != nil {
			return err
		}
		_, err = g.db.Exec(
			`UPDATE nodes SET outbound_json = ?, fingerprint = ? WHERE id = ?`,
			outboundJSON,
			outboundFingerprint(outboundJSON),
			item.id,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
