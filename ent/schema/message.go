package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type Message struct {
	ent.Schema
}

func (Message) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (Message) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(newUUIDv7),
		field.UUID("chat_id", uuid.UUID{}),
		field.UUID("sender_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("reply_to_id", uuid.UUID{}).Optional().Nillable(),
		field.Enum("type").
			Values(
				"regular",
				"system_create",
				"system_rename",
				"system_description",
				"system_avatar",
				"system_join",
				"system_add",
				"system_leave",
				"system_kick",
				"system_promote",
				"system_demote",
			).
			Default("regular"),
		field.Text("content").Optional().Nillable(),
		field.JSON("action_data", map[string]interface{}{}).
			Optional(),
		field.Time("deleted_at").Optional().Nillable(),
		field.Time("edited_at").Optional().Nillable(),
	}
}

func (Message) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("chat", Chat.Type).Ref("messages").Field("chat_id").Unique().Required(),
		edge.From("sender", User.Type).Ref("sent_messages").Field("sender_id").Unique(),
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
