# Jeepity

Jeepity is a Telegram bot powered by [OpenAI](https://openai.com) GPT large language models.

* **AI chatbot.** Interact with GPT 3.5/4 models in a private chat. The bot brings a ChatGPT-like experience to
  Telegram: it keeps
  the context of the conversation, displays the responses word-by-word as they are generated, and supports Markdown.
  messages.
* **Voice message transcription.** Forward someone else's voice message to the bot, and it will be transcribed using
  the [OpenAI Whisper](https://openai.com/research/whisper) model.
  You can also talk to the chatbot via voice messages: the transcriptions will be sent to the language model.
* **Localisation.** Buttons, menus, and system messages are available in English and Russian. The language is
  selected automatically based on the Telegram interface language.
* **Easy to self-host.** The bot compiles into a single binary and is also available as a Docker
  container. Check out the [tutorial](#deploying-to-flyio) on how to get it up and running in minutes
  on [fly.io](https://fly.io)!

## Quick Start

### Docker

* Create a file named `.env` with the configuration (see [Configuration](#configuration))
* Run `docker run --env-file .env ghcr.io/mkuznets/jeepity:latest`

### Docker Compose

* Copy the provided [compose.yml](compose.yaml)
* Customise the environment variables (see [Configuration](#configuration))
    * You can set them in the shell environment, in a `.env` file, or directly in the compose.yaml. See
      the [documentation](https://docs.docker.com/compose/environment-variables/set-environment-variables/) for more
      details.
* Run `docker-compose up`

### From Source

```shell
# Requires Go 1.19+
$ go install github.com/mkuznets/jeepity/cmd/jeepity@latest
$ jeepity run --help
```

### Pre-Built Binaries

> Coming soon!

## Configuration

Jeepity can be configured using environment variables
(recommended), .env file, or CLI arguments.
Check `jeepity run --help` to see the list of available options.

```dotenv
## Required

# OpenAI API key (https://platform.openai.com/account/api-keys)
OPENAI_TOKEN=sk-...
# Telegram bot token (https://core.telegram.org/bots#how-do-i-create-a-bot)
TELEGRAM_BOT_TOKEN=
# Local directory used to store the bot's database
DATA_DIR=./data

## Optional, uncomment if needed.

## Customise the chat completion model (default: gpt-3.5-turbo-0301)
#OPENAI_CHAT_MODEL=gpt-4-0314

## Customise the audio transcription model (default: whisper-1)
#OPENAI_AUDIO_MODEL=whisper-1

## Customise the password used to encrypt chat messages.
## If not set, the messages will still be encrypted with an empty password.
#DATA_ENCRYPTION_PASSWORD=
```

## Usage

### Access

To avoid abuse and excessive OpenAI API bills, access to the bot is invite-only. Every bot user can run the `/invite`
command to get a shareable access link that looks like this:

```
https://t.me/<username>?start=<code>
```

Opening the link is equivalent to running the `/start <code>` command manually (the link may not always work on
mobile devices).

When you run Jeepity for the first time, use the URL/code printed in the logs to get the access:

```
time=... level=DEBUG msg="Starting Telegram bot..."
time=... level=INFO msg="Invite URL: https://t.me/<username>?start=<code>"
```

Note that it is regenerated every time you restart the app, do not share it publicly!

### Chatbot

Use the private chat with the bot to talk to the language model. The bot will keep the context of the conversation
until:

* you run the `/reset` command to start a new conversation,
* there are no new messages for 1 hour,
* or the context exceeds the limit of the language model (you will be prompted to reset the conversation).

### Voice Message Transcription

When you forward someone else's voice message to the bot, it will be transcribed using the OpenAI Whisper model. You can
also record you own voice message to the bot, in which case the transcription will be part of the chatbot conversation.

Note that forwarding a voice message from the "Saved Messages" chat will count as a newly recorded message, and will
trigger the chatbot response.

## Self-Hosting

Some operational details:

* The Telegram bot uses long-polling, so you do not need to expose it to the internet and set up a webhook.
* Jeepity emits structured key-value logs to stderr. The is either human-readable [logfmt](https://brandur.org/logfmt)
  or JSON, depending on whether the shell session is interactive.
* Jeepity gracefully handles SIGTERM and SIGINT: it stops accepting new requests, waits for the ongoing requests to
  finish, and only then shuts down. You can force the shutdown by sending one or two extra SIGTERM/SIGINT.
* The data (users, chat history, etc.) is stored in a local SQLite database at `DATA_DIR`.
* The messages are encrypted with AES using the configured password (`DATA_ENCRYPTION_PASSWORD`) and a
  random per-user salt. When a conversation is reset (either manually or automatically), the messages are deleted.

## Deploying to fly.io

> I am not affiliated with fly.io in any way, just geniunely excited about how easy it is to deploy Jeepity there.

[Fly.io](https://fly.io) is a Platform as a Service where most of the resources (apps, machines, volumes, etc.) are
controlled using a command-line tool ([flyctl](https://fly.io/docs/flyctl)).
However, with their new [Web CLI](https://community.fly.io/t/introducing-fly-io-terminal/10464) you do not even need to
install anything!

Jeepity works perfectly on their smallest instances (shared-cpu-1x 256mb) included in the free allowance (as long as you
are eligible for it).

Follow these steps to spin up a new app running Jeepity:

1. Obtain an [OpenAI API key](https://platform.openai.com/account/api-keys) and
   a [Telegram bot token](https://core.telegram.org/bots#how-do-i-create-a-bot).
2. [Create a fly.io account](https://fly.io/app/sign-up).
3. Go to [fly.io/terminal](https://fly.io/terminal) and click "Launch Web CLI".
4. Run the following commands, one by one:

```shell
# Download the app template
curl -fsSL https://raw.githubusercontent.com/mkuznets/jeepity/master/template.fly.toml >fly.toml

# Create a fly app with an auto-generated name
fly launch --generate-name --copy-config --force-machines --no-deploy

# Create a 1GB volume to store the bot's database
fly volumes create --region lhr --size 1 --yes jeepity_data
```

5. Configure Jeepity by running `fly secrets import`. The command should hang waiting for your input. Paste the
   following text (fill the token values from step 1) and press Ctrl+D:

```dotenv
OPENAI_TOKEN=...
TELEGRAM_BOT_TOKEN=...
```

6. Run `fly deploy` to start the deploy.
7. Go to the "Watch your app" URL. You should see something like this:

```dotenv
[info]{"time":"...","level":"DEBUG","msg":"Starting Telegram bot..."}
[info]{"time":"...","level":"INFO","msg":"Invite URL: https://t.me/<username>?start=<code>"}
[info]{"time":"...","level":"INFO","msg":"(This URL is temporary, DO NOT SHARE IT WITH ANYONE)"}
```

8. The bot is up and running! Go to the invite URL to start using the bot!

## License

Jeepity is released under the MIT License. See the [LICENSE](LICENSE) file for more details.
