# Tools

## Agent Tools
These are the tools available for the agent to perform tasks.

### Filesystem
Manages files and directories within allowed locations. You can combine these functions to perform complex tasks. All paths must be within permitted directories.

**Available functions:**
- `read_file`: Reads the entire content of a single specified file.
- `read_multiple_files`: Reads contents of several files at once.
- `write_file`: Creates a new file or overwrites an existing one with provided content.
- `edit_file`: Performs line-based edits on a text file.
- `create_directory`: Creates a new directory.
- `list_directory`: Lists all files and subdirectories in a specified directory.
- `directory_tree`: Provides a recursive JSON tree view of files and directories.
- `move_file`: Moves or renames files/directories.
- `search_files`: Recursively searches for files/directories matching a case-insensitive pattern.
- `get_file_info`: Retrieves detailed metadata for a file or directory.
- `list_allowed_directories`: Shows the list of directories this tool can access.
- `delete_path`: Deletes a specified file or directory.

### Execute Python
Executes Python code and returns the result. This tool supports installing additional packages and stdin input.

### Bash
Executes a bash command. Use this tool to run existing scripts, system commands, or manage processes. Examples: `python script.py`, `ls -la`, `curl ...`. It runs in the system shell.

## User Commands
- **/start**: Welcome message
- **/about**: Info about Myaaw
- **/me**: User info and config
- **/models**: Change LLM model
- **/reset**: Reset context window
