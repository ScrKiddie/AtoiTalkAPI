package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type User struct {
	ent.Schema
}

func (User) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").MaxLen(255).Unique().NotEmpty(),
		field.String("password_hash").MaxLen(255).Optional().Nillable().Sensitive(),
		field.String("full_name").MaxLen(100).NotEmpty(),
		field.String("bio").MaxLen(255).Optional().Nillable(),
		field.Int("avatar_id").Optional().Nillable(),
		field.Bool("is_online").Default(false),
		field.Time("last_seen_at").Optional().Nillable(),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("avatar", Media.Type).
			Ref("user_avatar").
			Unique().
			Field("avatar_id"),

		edge.To("identities", UserIdentity.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("sent_messages", Message.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("created_groups", GroupChat.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("group_memberships", GroupMember.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("private_chats_as_user1", PrivateChat.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("private_chats_as_user2", PrivateChat.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
