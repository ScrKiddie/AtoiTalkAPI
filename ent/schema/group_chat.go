package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type GroupChat struct {
	ent.Schema
}

func (GroupChat) Fields() []ent.Field {
	return []ent.Field{
		field.Int("chat_id").Unique(),
		field.Int("created_by"),
		field.String("name").MaxLen(100).NotEmpty(),
		field.Text("description").Optional().Nillable(),
		field.Int("avatar_id").Optional().Nillable(),
	}
}

func (GroupChat) Edges() []ent.Edge {
	return []ent.Edge{

		edge.From("avatar", Media.Type).
			Ref("group_avatar").
			Unique().
			Field("avatar_id"),

		edge.From("chat", Chat.Type).Ref("group_chat").Field("chat_id").Unique().Required(),
		edge.From("creator", User.Type).Ref("created_groups").Field("created_by").Unique().Required(),
		edge.To("members", GroupMember.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
