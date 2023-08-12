-- Add column "system_prompt" to table: "users"
ALTER TABLE `users` ADD COLUMN `system_prompt` text NULL;
-- Add column "input_state" to table: "users"
ALTER TABLE `users` ADD COLUMN `input_state` text NULL;
