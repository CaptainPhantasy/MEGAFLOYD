# Company platform

This service gives company members their own accounts and connects the coding app to shared models and tools. The owner manages accounts, provider details and the administrator key from the administration page.

The service uses the Python standard library. Run `python3 test_platform.py` to check account boundaries, administrator key changes, model replies and tool inputs.

Keep `config.json`, the administrator key, account records and uploads outside source control. Existing service installations retain those files when these source files are updated. Serve the private local listener through the company’s secure network and existing web server.

`company-tools.mjs` runs through the coding app’s built-in program runner and uses the signed-in member’s saved access. It never includes the owner’s server credentials.

This is a testing checkpoint. The portable app’s built-in command window still needs its missing files added. Browser reply timing and reconnection checks remain unfinished. The included speech and music work is retained from this checkpoint; no model training is included.
