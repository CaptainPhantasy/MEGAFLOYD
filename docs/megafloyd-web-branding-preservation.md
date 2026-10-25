# MegaFloyd customized web bundle preservation

The browser frontend source is absent from this checkout. Its customized application is the prebuilt bundle at `/Volumes/SanDisk1Tb/MEGAFLOYD/floyd-code/apps/floyd-code/dist-web`. The desktop wrapper has source under `/Volumes/SanDisk1Tb/MEGAFLOYD/desktop/desktop/src`.

The ordinary build compiles the server and worker into `dist`. The prebuild visualization task writes a separate visualization asset; the native-helper copy only replaces native helpers. `clean` removes `dist`. None of those commands regenerates `dist-web`.

The earlier check only required `index.html`. It now verifies exact file names and SHA-256 hashes against `web-assets-manifest.json`, before and after the ordinary build. Missing, modified, additional, or nonregular frontend assets fail the check. Dated backup files and `.DS_Store` are excluded consistently. Builds do not rewrite the manifest to accept arbitrary changes.

At the initial 22:49 checkpoint, the manifest protected all **560 canonical customized frontend files**. Their contents were unchanged by this guard. The direct check accepted the actual bundle and a copied bundle, rejected an altered mascot in a temporary copy, and confirmed the canonical mascot was unchanged. No frontend rebuild was run.

The desktop packager now runs the same check against the staged frontend before assembly. The actual 224951 desktop assembly verified all 560 assets before copying and packaging them. Its build log and installed inventory retain the unchanged frontend tree hash.

Receipt: `megafloyd-web-branding-preservation.json`. Initial guard verification: 24 October 2026, 22:49:10 UTC. Existing package receipts separately record the frontend tree hash `a177bbf3773cc381b455df845ab1698f575d186e7a90f12b131526aa6dcd3080`.

## Authorized artwork amendment and installed package

The final installed release `megafloyd-20261024T234343Z` contains **561 checked frontend assets**. The owner-authorized wordmark and founder-head revisions were made in the maintained bundle and explicitly recorded in the manifest. The ordinary build did not regenerate or replace the custom frontend. The original sprite artwork is unchanged.

The clean implementation commit is `8c4876a3d166d9ac1ddd9952f27b2ab0bed3dfd3`. Package verification at 23:44:42 UTC matched the complete asset tree (`3d08c82a3ee19c84e07802a3e1e3d97d9871a0b7a922c230e9551590b4254083`) and all 2,061 inventory entries. Subsequent actual installed browser checks verified the larger heads, pointer directions, wordmark animation and live session. The earlier 560-file receipt remains historical evidence of the guard implementation.

The review-fix package `megafloyd-20261024T235658Z`, built from clean commit `67deb1ae9723e77d7cdc52b3ce23fb430a7af509`, preserves the same 561 artwork files and manifest byte for byte. Its server bundle includes the six review corrections; complete package verification passed before installation.
