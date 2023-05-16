-- Create "users" table
CREATE TABLE `users` (`chat_id` integer NOT NULL, `approved` integer NOT NULL DEFAULT 1, `username` text NOT NULL, `full_name` text NOT NULL, `created_at` integer NOT NULL, `updated_at` integer NOT NULL, `salt` text NOT NULL, PRIMARY KEY (`chat_id`), CHECK (created_at > 0), CHECK (updated_at > 0)) strict;
-- Create index "users_chat_id_idx" to table: "users"
CREATE UNIQUE INDEX `users_chat_id_idx` ON `users` (`chat_id`);
-- Create "messages" table
CREATE TABLE `messages` (`id` integer NULL, `chat_id` integer NOT NULL, `role` text NOT NULL, `message` text NOT NULL, `version` integer NOT NULL, `created_at` integer NOT NULL, PRIMARY KEY (`id`), CONSTRAINT `chat_id` FOREIGN KEY (`chat_id`) REFERENCES `users` (`chat_id`) ON UPDATE NO ACTION ON DELETE CASCADE, CHECK (created_at > 0)) strict;
-- Create index "messages_chat_id_idx" to table: "messages"
CREATE INDEX `messages_chat_id_idx` ON `messages` (`chat_id`);
-- Create "usage" table
CREATE TABLE `usage` (`id` integer NULL, `chat_id` integer NOT NULL, `update_id` integer NOT NULL, `model` text NOT NULL, `completion_tokens` integer NOT NULL, `prompt_tokens` integer NOT NULL, `total_tokens` integer NOT NULL, `created_at` integer NOT NULL, PRIMARY KEY (`id`), CONSTRAINT `chat_id` FOREIGN KEY (`chat_id`) REFERENCES `users` (`chat_id`) ON UPDATE NO ACTION ON DELETE CASCADE, CHECK (created_at > 0)) strict;
-- Create index "usage_chat_id_created_at_idx" to table: "usage"
CREATE INDEX `usage_chat_id_created_at_idx` ON `usage` (`chat_id`, `created_at`);
