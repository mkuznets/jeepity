# Jeepity: Telegram Bot for OpenAI GPT Models

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
  container. Check out the tutorial on how to get it up and running in minutes on [fly.io](https://fly.io)!

## Installation

### Docker

* Create a `.env` file with the configuration (see [Configuration](#configuration))
* Run `docker run --env-file .env public.ecr.aws/mkuznets/jeepity:latest`

### Docker Compose

* Copy the provided [compose.yml](compose.yaml)
* Customise the environment variables (see [Configuration](#configuration))
    * You can set them in the shell environment, in a `.env` file, or directly in the compose.yaml. See
      the [documentation](https://docs.docker.com/compose/environment-variables/set-environment-variables/) for more
      details.
* Run `docker-compose up`

## Configuration

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

## License

Jeepity is released under the MIT License. See the [LICENSE](LICENSE) file for more details.
