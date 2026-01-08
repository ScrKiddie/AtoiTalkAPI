package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type Chat struct {
	ent.Schema
}

func (Chat) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (Chat) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(newUUIDv7),
		field.Enum("type").Values("private", "group").Immutable(),
		field.UUID("last_message_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("last_message_at").Optional().Nillable(),
		field.Time("deleted_at").Optional().Nillable(),
	}
}

func (Chat) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("messages", Message.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("private_chat", PrivateChat.Type).Unique().Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("group_chat", GroupChat.Type).Unique().Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("last_message", Message.Type).Unique().Field("last_message_id"),
	}
}

func (Chat) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("updated_at"),
		index.Fields("last_message_at"),
	}
}
