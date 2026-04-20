package switchboard

import (
	"context"
	"database/sql"
	"time"

	"github.com/dmilov/jacquard/internal/models"
)

type DB struct {
	db *sql.DB
}

func NewDB(db *sql.DB) *DB { return &DB{db: db} }

func (d *DB) SaveConversation(ctx context.Context, conv models.Conversation) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT IGNORE INTO conversations (id, node_id, command, started_at) VALUES (?, ?, ?, ?)`,
		conv.ID, conv.NodeID, conv.Command, conv.StartedAt,
	)
	return err
}

func (d *DB) EndConversation(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE conversations SET ended_at=? WHERE id=?`,
		time.Now().UTC(), id,
	)
	return err
}

func (d *DB) ListConversations(ctx context.Context, nodeID string) ([]models.Conversation, error) {
	query := `SELECT id, node_id, command, started_at, ended_at
	          FROM conversations ORDER BY started_at DESC LIMIT 200`
	args := []any{}
	if nodeID != "" {
		query = `SELECT id, node_id, command, started_at, ended_at
		         FROM conversations WHERE node_id=? ORDER BY started_at DESC LIMIT 200`
		args = append(args, nodeID)
	}
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convs []models.Conversation
	for rows.Next() {
		var c models.Conversation
		if err := rows.Scan(&c.ID, &c.NodeID, &c.Command, &c.StartedAt, &c.EndedAt); err != nil {
			return nil, err
		}
		convs = append(convs, c)
	}
	return convs, rows.Err()
}

func (d *DB) GetMessages(ctx context.Context, conversationID string) ([]models.Message, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, conversation_id, role, content, sequence, created_at
		 FROM messages WHERE conversation_id=? ORDER BY sequence ASC`,
		conversationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []models.Message
	for rows.Next() {
		var m models.Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Sequence, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}
