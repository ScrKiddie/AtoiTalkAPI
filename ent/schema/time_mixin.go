package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

type TimeMixin struct {
	mixin.Schema
}

func (TimeMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").
			Immutable().
			Default(nowUTC).
			Annotations(entsql.Default("CURRENT_TIMESTAMP")),
		field.Time("updated_at").
			Default(nowUTC).
			UpdateDefault(nowUTC).
			Annotations(entsql.Default("CURRENT_TIMESTAMP")),
	}
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
