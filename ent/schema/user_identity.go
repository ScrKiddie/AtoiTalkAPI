package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type UserIdentity struct {
	ent.Schema
}

func (UserIdentity) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (UserIdentity) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(newUUIDv7),
		field.UUID("user_id", uuid.UUID{}),
		field.Enum("provider").Values("google").Default("google"),
		field.String("provider_id").MaxLen(255).NotEmpty(),
		field.String("provider_email").MaxLen(255).Optional().Nillable(),
	}
}

func (UserIdentity) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("identities").
			Unique().
			Required().
			Field("user_id"),
	}
}

func (UserIdentity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "provider_id").Unique(),
		index.Fields("user_id", "provider").Unique(),
	}
}
