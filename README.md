# art.media.platform.SDK
_A fully provisioned solution for files, media, and 3D asset sharing and deployment we can all agree on._

**_art.media.platform_** "Amp" is a potent 3D client-to-infrastructure aid that provides a secure, scalable, and extensible runtime for 3D applications. It supports 3D and media-centric apps with pluggable infrastructure, and allows artists, publishers, creators, and organizations to control asset deployments and experiences within high-fidelity spatial or geographic environments.

### Key Features

- Secure, "turn-key" support for:
  - Distributing _spatially_ organized media and files, transforming human accessibility.
  - Browsing and previewing assets and content for public or private consumption or sale.
  - Publishing first-class 3D experiences on _Windows_, _Mac_, _Linux_, _Android_, _iOS_, and XR headsets like _Oculus_ and _VisionPro_.
  - Deploying content "crates" independently of your app release cycle.
  - Integrated hardware-based authentication & signing (e.g. [Yubikey](https://yubico.com))

- Direct integration with **[Unity](https://unity.com)** and **[Unreal](https://unreal.com)** by embedding Amp's **[Go](https://golang.org)** native library that your 3D app invokes through convenient bindings.

- A lightweight, stand-alone "headless" executable named `amp.host` that offers federated and decentralized support and storage options.

## _What does this solve?_

***Amp bridges native 3D apps to system, network, and infrastructure services, solving key problems.***

Traditional file and asset management systems are inadequate when there are hundreds or thousands of assets to organize, experience, or review. Teams often resort to makeshift solutions for collaboration and sharing which compromise efficiency and security.

Teams often collaborate over large file sets, yet they deploy using production systems that are entirely different from their development workflows. Many sharing and collaboration solutions exist, but they lack first-class spatial linking and native 3D content integration while suffering from inflexible, confining web or OS-based user experiences.

Meanwhile, _web-based_ 3D frameworks such as [three.js](https://threejs.org/) do not compare to _native_ Unreal and Unity experiences and offer no path for real-world asset deployments. For example, 3D experiences require asset deployments often exceeding _many gigabytes_ and are impossible through a web-based approach. Worse, _web stacks pose many blockers that publishers have little or no ability to address, such as texturing features, performance potholes, scene management, and AI support._

***art.media.platform*** is a bridge and toolbox that allows 3D app developers to focus on their core value proposition. It offers rich support for persistent state, user interfaces, and content immersion and allows you to break free of limiting web or OS infrastructure. _Teams, leads, designers, artists, organizers, and ultimately consumers need better tools to richly and safely share assets._

### Spatial Web

This stack makes infrastructure more accessible through spatial idioms — and web is no exception. Amp offers support for integrated, in-app web browsing that pairs powerfully with spatial linking. Frameworks such as [Webview](https://developer.vuplex.com/webview/overview) are just another component in the Amp client, allowing your app to have an embedded web browser out of the box. This allows URLs and web experiences to be linked spatially or from multiple map locations.

### Geo/Spatial Linking

Geographic and spatial-centric applications such as GIS, CAD, and BIM, are everywhere in modern construction, contracting, and real-time logistics. Amp's 3D client natively integrates [maps and locations](https://infinity-code.com/assets/online-maps), allowing you to unify location-based linking, spatially precise environments, and first-class 3D asset integration.

### Extensibility

The less obvious value of Amp is its _extensibility_. The [`amp.App`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.app.go) interface is flexible and unrestricted, allowing you to expose anything compatible with Go. This means any Go, C, C++, or any native static or dynamic module can be wrapped and push a 3D-native UX (with stock or custom assets).

## Integration Flow

This repo is lightweight and dependency-free so it can be added to your project without consequence.

At a high level:

1. Add [amp.SDK](https://github.com/art-media-platform/amp.SDK) to your Go project. If you want to expose additional functionality, implement your own [`amp.App`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.app.go).
2. Clone [amp.host](https://github.com/art-media-platform/amp.host) (not yet public) and include your `amp.App`, similar to how a library in a C project registers a static or dynamic dependency.
3. Build `amp.host` with your additions embedded within it.
4. In your Unity or Unreal app, link in `amp.host.lib` and add the Amp UX runtime support glue.
5. On startup, [`amp.Host`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.host.go) instantiates registered `amp.App` instances as needed. During runtime, `amp.host.lib` dispatches URL requests addressed to your app and are "pinned".
6. The Amp UX runtime manages the user's experience of currently pinned URLs while providing a toolbox of extendable "stock" and "skinnable" components. Pinned requests receive state updates until they are canceled.

## Previous Work

[Previous generations](https://github.com/plan-systems/plan-go/tags) of this framework went into production in 2019 to become [PLAN 3D](https://plan-systems.org/). This overall [architecture](https://github.com/plan-systems/design-docs), though ambitious, continues to accurately describe many purposes of this framework.

In a world increasingly vulnerable to AI-assisted exploits, Amp's [security model](https://github.com/plan-systems/design-docs/blob/master/PLAN-Proof-of-Correctness.md) prioritizes security and privacy. It uses a container-based approach to ensure state-grade protection while the client runtime supports rich, native 3D experiences.

## Points of Interest

|                                                                                                   |                                                                                                                                                                                 |
| ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [api.tag.go](https://github.com/art-media-platform/amp.SDK/blob/main/stdlib/tag/api.tag.go)    | versatile tagging and hash scheme that is AI and search friendly                                                                                                                  |
| [api.task.go](https://github.com/art-media-platform/amp.SDK/blob/main/stdlib/task/api.task.go) | goroutine wrapper inspired by a conventional parent-child process model                                                                                                    |
| [api.app.go](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.app.go)           | defines how state is requested, pushed, and merged                                                                                              |
| [api.host.go](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.host.go)         | types and interfaces that [`amp.host`](https://github.com/art-media-platform/amp.host) / `amp.host` implements                                                              |
