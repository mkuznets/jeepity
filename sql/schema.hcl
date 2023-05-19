table "users" {
  schema = schema.main
  column "chat_id" {
    null = false
    type = integer
  }
  column "approved" {
    null    = false
    type    = integer
    default = 1
  }
  column "username" {
    null = false
    type = text
  }
  column "full_name" {
    null = false
    type = text
  }
  column "created_at" {
    null = false
    type = integer
  }
  column "updated_at" {
    null = false
    type = integer
  }
  column "salt" {
    null = false
    type = text
  }
  column "model" {
    null = true
    type = text
  }

  primary_key {
    columns = [column.chat_id]
  }
  index "users_chat_id_idx" {
    unique  = true
    columns = [column.chat_id]
  }

  check {
    expr = "(created_at > 0)"
  }
  check {
    expr = "(updated_at > 0)"
  }
}

table "messages" {
  schema = schema.main
  column "id" {
    null = true
    type = integer
  }
  column "chat_id" {
    null = false
    type = integer
  }
  column "role" {
    null = false
    type = text
  }
  column "message" {
    null = false
    type = text
  }
  column "version" {
    null = false
    type = integer
  }
  column "created_at" {
    null = false
    type = integer
  }

  primary_key {
    columns = [column.id]
  }
  foreign_key "chat_id" {
    columns     = [column.chat_id]
    ref_columns = [table.users.column.chat_id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
  index "messages_chat_id_idx" {
    columns = [column.chat_id]
  }

  check {
    expr = "(created_at > 0)"
  }
}

table "usage" {
  schema = schema.main
  column "id" {
    null = true
    type = integer
  }
  column "chat_id" {
    null = false
    type = integer
  }
  column "update_id" {
    null = false
    type = integer
  }
  column "model" {
    null = false
    type = text
  }
  column "completion_tokens" {
    null = false
    type = integer
  }
  column "prompt_tokens" {
    null = false
    type = integer
  }
  column "total_tokens" {
    null = false
    type = integer
  }
  column "created_at" {
    null = false
    type = integer
  }

  primary_key {
    columns = [column.id]
  }
  foreign_key "chat_id" {
    columns     = [column.chat_id]
    ref_columns = [table.users.column.chat_id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
  index "usage_chat_id_created_at_idx" {
    columns = [column.chat_id, column.created_at]
  }

  check {
    expr = "(created_at > 0)"
  }
}

schema "main" {}