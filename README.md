# CPE DB Aggregator [![Fetch NIST CPEs](https://github.com/oneaudit/cpedb-aggregator/actions/workflows/main.yaml/badge.svg)](https://github.com/oneaudit/cpedb-aggregator/actions/workflows/main.yaml)

### Description 🗺️

The [NIST](https://nvd.nist.gov/developers/products) **has deprecated the CPE XML feed** as of the end of 2023.<br>
This repository provides a daily cache of their API, updated every day at 4:00 AM.

```
> Refer to `.github/workflows/main.yaml`.
> Refer to the `update` branch.
```

### How it works? ✍️

We generate **one JSON file per CPE entry**. While this increases the repository size, the design enables clients to perform **differential updates** — applying logic only to modified files.

```
> For example, a client that parses and caches the data
> will only need to update changed JSON files instead of the entire database.
```

### Optimization 🚀

After each update, we store the timestamp of the last successful sync.
This allows us to fetch **only the changes** since that point, broken into **120-day intervals** (due to API limitations).

```
> See `.update_date` for the latest recorded update.
```

### License 📄

This project is licensed under the MIT License.<br>
You are free to use, modify, and distribute this software with proper attribution. See the [LICENSE](LICENSE) file for full details.
