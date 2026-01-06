package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type GroupMember struct {
	ent.Schema
}

func (GroupMember) Fields() []ent.Field {
	return []ent.Field{
		field.Int("group_chat_id"),
		field.Int("user_id"),
		field.Enum("role").Values("owner", "admin", "member").Default("member"),
		field.Time("last_read_at").Optional().Nillable(),
		field.Time("joined_at").Default(nowUTC).Immutable(),
		field.Int("unread_count").Default(0),
	}
}

func (GroupMember) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("group_chat", GroupChat.Type).Ref("members").Field("group_chat_id").Unique().Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.From("user", User.Type).Ref("group_memberships").Field("user_id").Unique().Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (GroupMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_chat_id", "user_id").Unique().StorageKey("pk_group_member"),
	}
}
