create table if not exists users
(
    chat_id    integer primary key not null,
    approved   integer             not null default 1,
    username   text                not null,
    full_name  text                not null,
    created_at integer             not null check (created_at > 0),
    updated_at integer             not null check (updated_at > 0),
    salt       text                not null
) strict;

create unique index if not exists users_chat_id_idx on users (chat_id);

create table if not exists messages
(
    id         integer primary key,
    chat_id    integer not null references users (chat_id) on delete cascade,
    role       text    not null,
    message    text    not null,
    created_at integer not null check (created_at > 0)
) strict;

create index if not exists messages_chat_id_idx on messages (chat_id);

create table if not exists usage
(
    id                integer primary key,
    chat_id           integer not null references users (chat_id) on delete cascade,
    update_id         integer not null,
    model             text    not null,
    completion_tokens integer not null,
    prompt_tokens     integer not null,
    total_tokens      integer not null,
    created_at        integer not null check (created_at > 0)
) strict;

create index if not exists usage_chat_id_created_at_idx on usage (chat_id, created_at);
