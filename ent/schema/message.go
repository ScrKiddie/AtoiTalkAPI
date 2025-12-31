package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Message struct {
	ent.Schema
}

func (Message) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (Message) Fields() []ent.Field {
	return []ent.Field{
		field.Int("chat_id"),
		field.Int("sender_id"),
		field.Int("reply_to_id").Optional().Nillable(),
		field.Text("content").Optional().Nillable(),
		field.Time("deleted_at").Optional().Nillable(),
		field.Bool("is_edited").Default(false),
	}
}

func (Message) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("chat", Chat.Type).Ref("messages").Field("chat_id").Unique().Required(),
		edge.From("sender", User.Type).Ref("sent_messages").Field("sender_id").Unique().Required(),
		edge.To("reply_to", Message.Type).Field("reply_to_id").Unique().From("replies").
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.To("attachments", Media.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (Message) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("chat_id", "created_at").
			Annotations(
				entsql.Desc(),
				entsql.IndexWhere("deleted_at IS NULL"),
			).
			StorageKey("idx_messages_chat_active"),
		index.Fields("reply_to_id").
			Annotations(entsql.IndexWhere("reply_to_id IS NOT NULL AND deleted_at IS NULL")),
	}
}
