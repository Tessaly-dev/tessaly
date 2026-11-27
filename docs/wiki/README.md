# Wiki pages

This folder holds the pages for the [GitHub wiki](https://github.com/tessaly-dev/tessaly/wiki). The repo is the source of truth. The wiki is a published copy.

## Keeping the wiki in sync

1. Edit pages here, never in the wiki web editor. Changes go through a pull request like code.
2. Change a page here in the same pull request that changes the behaviour, config, API or dev setup it describes. Add a page when a feature lands, not before. The PR template asks about this.
3. Until the sync Action exists, a maintainer copies the pages after each merge to `main`:

   ```sh
   git clone https://github.com/tessaly-dev/tessaly.wiki.git
   cp docs/wiki/*.md tessaly.wiki/
   rm tessaly.wiki/README.md
   cd tessaly.wiki && git add -A && git commit -s -m "docs: sync wiki from main" && git push
   ```

4. Once the wiki is turned on, a GitHub Action will push this folder to the wiki on every merge to `main`, with `contents: write` limited to that one job.
5. At each release, maintainers read the wiki for stale pages and open issues for them.

## File names

GitHub wiki pages take their title from the file name: `Dev-setup.md` becomes "Dev setup". `Home.md` is the start page and `_Sidebar.md` is the menu. Link between pages with the name and no extension, for example `[Dev setup](Dev-setup)`.

This README is not a wiki page and is not copied.
