# Foundry VTT Package CORS Proxy

A small, fast CORS proxy written in Go, for looking up Foundry VTT packages from the browser. Foundry's package listing, GitHub's release downloads and many manifest hosts send no CORS headers, so a module running in Foundry, such as [EMMA](https://github.com/Mondarth/EMMA), cannot read them directly. This proxy fetches them on the module's behalf and adds the headers.

It is a fork of [melihbirim/corsproxy](https://github.com/melihbirim/corsproxy), carrying on from its 1.2.6 release.

```{toctree}
:caption: 'For Users:'
:maxdepth: 2

user/index
```

```{toctree}
:caption: 'For Developers:'
:maxdepth: 2

developer/index
```

```{todolist}
```
