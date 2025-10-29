
# zimdex

Zimdex (`/ˈzɪm.dɛks/`) is an offline and self-hostable ZIM search engine that is built for in-Browser
usage. It offers a bundled Web interface and can operate on multiple ZIM files and is meant to replace
the `New Tab` page or to be integrated into the Browser URL bar as a Custom Search Engine.

### Warning: Experimental Software

At this point Zimdex is very experimental, and the listed Features are not implemented yet.
But this list is kind of the TODO list for now as the project progresses in development.


### Features

- [ ] Search with `site:domain.tld`
- [ ] Search with `namespace:identifier`
- [ ] Search with `keyword` to losely match Titles and URLs
- [ ] Search with `+keyword` to require a keyword match

- [ ] Bookmark search results (or `+1`) them so they appear on top for future queries

- [ ] Show "Didn't find anything? Try these:" at the bottom of Results Page
- [ ] Show "Search on DuckDuckGo" button
- [ ] Show "Search on Bing" button
- [ ] Show "Search on Google" button


### Usage

Start the zimdex binary on a specified port and you're ready to go. The program has to be executed
within the folder that contains the ZIM files.

```bash
cd /home/zim;
ls; # shows multiple zim files

# start webserver on port 80
zimdex --port=80;
```


### Browser Usage

On your server:

- Start zimdex on a reachable port (defaulted port `80` is recommended)

On your desktop machine:

- Use either the Hostname or IP of the server in the URL
- Add a custom search engine in Firefox `about:preferences#search`
- Set Search Engine Name to `zimdex`
- Set URL to `http://server_hostname_or_ip:80/search?q=%s` (replace hostname and port accordingly)
- Set Keyword to `@z` so you can use `@z example` to search in the URL bar


### License

This project is licensed under the [AGPL 3.0](./LICENSE.txt) License.

