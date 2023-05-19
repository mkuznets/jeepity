-- Add column "invite_code" to table: "users"
ALTER TABLE `users` ADD COLUMN `invite_code` text NULL;
-- Add column "invited_by" to table: "users"
ALTER TABLE `users` ADD COLUMN `invited_by` integer NULL;
-- Create index "users_invite_code_idx" to table: "users"
CREATE INDEX `users_invite_code_idx` ON `users` (`invite_code`);
