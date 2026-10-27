# Voice

Myaaw has two voice modes:

| Command | Pipeline | Requirements |
| :--- | :--- | :--- |
| `voice` | Real-time audio with Gemini Multimodal Live | Gemini provider + API key |
| `voice-classic` | Speech-to-text → agent loop (tools) → text-to-speech | Any LLM provider, plus transcriber and synthesizer keys |

## Build requirements

Both commands use native audio libraries through CGo:

**macOS**

```bash
brew install pkg-config portaudio
```

Grant Microphone, Screen Recording, and Camera permissions on first use.

**Linux (Ubuntu/Debian)**

```bash
sudo apt update
sudo apt install -y portaudio19-dev libasound2-dev pkg-config
# Screen/camera capture:
sudo apt install -y libx11-dev libxext-dev
```

**Windows**

Build with a CGo toolchain (MSYS2/MinGW):

```bash
pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-portaudio
```

## `myaaw voice`

Real-time, full-duplex conversation using the Gemini Live API.

```bash
myaaw voice
myaaw voice --video screen
myaaw voice --video screen,camera --video-interval 5
```

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--video` | `none` | Video source: `none`, `screen`, `camera`, or `screen,camera`. |
| `--video-interval` | `3` | Seconds between captured frames. |
| `--save-frames` | `false` | Save captured frames to `~/.myaaw/<agent>/logs/frames/`. |

Requirements:

- The active LLM provider must be **Gemini** (`provider default <gemini-name>`).
- That provider must have a valid API key.
- The command exits with an explanatory error if either is missing.

## `myaaw voice-classic`

Turn-based voice chat that runs the full agent loop, including tools.

```bash
myaaw voice-classic
myaaw voice-classic --video camera --save-audio
```

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--video` | `none` | Same sources as `voice`. |
| `--video-interval` | `3` | Seconds between captured frames. |
| `--save-frames` | `false` | Save frames to `logs/frames/`. |
| `--save-audio` | `false` | Save per-turn user and assistant audio to `logs/audio/`. |

Requirements:

```env
# ~/.myaaw/<agent>/.env
TRANSCRIBER_PROVIDER_NAME=<provider>
TRANSCRIBER_API_KEY=<key>
SYNTHESIZER_PROVIDER_NAME=<provider>
SYNTHESIZER_API_KEY=<key>
```

The LLM provider can be any configured provider.

## Examples

```bash
# Real-time voice with a specific agent
myaaw --agent agent-b voice

# Classic voice with camera frames every 5 seconds
myaaw --agent agent-b voice-classic --video camera --video-interval 5

# Record the session artifacts
myaaw voice-classic --save-audio --save-frames
```

## Notes

- Interactive voice needs microphone access. The command prints a clear error if PortAudio or the OS permission is missing.
- Frames and audio are written under the active agent's `logs/` directory.
- Transcripts are written to the agent's log directory by the voice commands.
- `voice` does not use tools; `voice-classic` does, because it runs the normal agent loop.

## Related

- [Providers](provider.md)
- [Multi-Agent Guide](../agents/multi-agent.md)
- [Agent Loop](../agents/agent-loop.md)
