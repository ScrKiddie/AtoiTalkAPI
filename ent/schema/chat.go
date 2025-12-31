package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Chat struct {
	ent.Schema
}

func (Chat) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (Chat) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("type").Values("private", "group").Immutable(),
		field.Int("last_message_id").Optional().Nillable(),
		field.Time("last_message_at").Optional().Nillable(),
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
