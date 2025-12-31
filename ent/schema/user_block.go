package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UserBlock struct {
	ent.Schema
}

func (UserBlock) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (UserBlock) Fields() []ent.Field {
	return []ent.Field{
		field.Int("blocker_id"),
		field.Int("blocked_id"),
	}
}

func (UserBlock) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("blocker", User.Type).
			Ref("blocked_users_rel").
			Field("blocker_id").
			Unique().
			Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.From("blocked", User.Type).
			Ref("blocked_by_rel").
			Field("blocked_id").
			Unique().
			Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (UserBlock) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("blocker_id", "blocked_id").Unique(),
	}
}
