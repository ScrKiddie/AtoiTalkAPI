package schema

import (
	"context"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type PrivateChat struct {
	ent.Schema
}

func (PrivateChat) Fields() []ent.Field {
	return []ent.Field{
		field.Int("chat_id").Unique(),
		field.Int("user1_id"),
		field.Int("user2_id"),
		field.Time("user1_last_read_at").Optional().Nillable(),
		field.Time("user2_last_read_at").Optional().Nillable(),
	}
}

func (PrivateChat) Hooks() []ent.Hook {
	return []ent.Hook{
		func(next ent.Mutator) ent.Mutator {
			return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
				if m.Op().Is(ent.OpCreate) {
					v1, _ := m.Field("user1_id")
					v2, _ := m.Field("user2_id")
					id1, id2 := v1.(int), v2.(int)
					if id1 > id2 {
						m.SetField("user1_id", id2)
						m.SetField("user2_id", id1)
					}
				}
				return next.Mutate(ctx, m)
			})
		},
	}
}

func (PrivateChat) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("chat", Chat.Type).Ref("private_chat").Field("chat_id").Unique().Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.From("user1", User.Type).Ref("private_chats_as_user1").Field("user1_id").Unique().Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.From("user2", User.Type).Ref("private_chats_as_user2").Field("user2_id").Unique().Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (PrivateChat) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user1_id", "user2_id").Unique().StorageKey("unique_user_pair"),
	}
}
