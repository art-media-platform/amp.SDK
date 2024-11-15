# art.media.platform.SDK
_A fully provisioned solution for files, media, and 3D asset sharing and deployment we can all agree on._

**_art.media.platform_** ("Amp") is a potent 3D client-to-infrastructure suite that provides a secure, scalable, and extensible runtime for 3D applications. It supports 3D and media-centric apps with pluggable infrastructure, allowing artists, publishers, creators, and organizations to control asset deployments and experiences within high-fidelity spatial or geographic environments.

## Key Features

- Secure, "turn-key" support for:
  - __3D spaces and linking__: Users experience _spatially_ placed media and files and can be linked to any real or virtual space, _transforming human accessibility_.
  - __Payment processing__: Built-in payment suite **amp.nile** offers easy integration with [Stripe](https://stripe.com/) and [Payflow](https://developer.paypal.com/api/nvp-soap/payflow/payflow-gateway/).
  - __Platform coverage__: Amp lets you ship first-class 3D experiences on _Windows_, _Mac_, _Linux_, _Android_, _iOS_, and AR/VR ("XR") platforms like _VisionPro_, _Horizon_, and other OEM ecosystems (e.g. HoloLens, Magic Leap).
  - __Continuous deployment__: Amp's "crate" system provides asset and SKU independence from your marketing and engineering release cycles.
  - __Integrated security__: Full support for third-party providers and hardware-based authentication & signing (e.g., [Yubikey](https://yubico.com)).

- Seamless integration with **[Unity](https://unity.com)** and **[Unreal](https://unrealengine.com)** via an embedded **[Go](https://golang.org)** native library that your 3D app invokes through convenient bindings — available in the **amp.SDK**.

- A lightweight, stand-alone "headless" native executable and shared library **amp.host.lib** with tags **amp.host** that offers federated and decentralized service and storage.

## What Does This Solve?

***Amp bridges native 3D apps to system, network, and infrastructure services, addressing key challenges:***

Traditional file and asset management systems are inadequate for organizing, experiencing, or reviewing hundreds or thousands of assets. Teams often resort to makeshift solutions for collaboration and sharing, compromising efficiency and security.

Teams often collaborate over large file sets but deploy using production systems entirely different from their development workflows. Many sharing and collaboration solutions exist, but they lack first-class spatial linking and native 3D content integration while suffering from inflexible, confining web or OS-based user experiences.

Meanwhile, _web-based_ 3D frameworks like [Three.js](https://threejs.org/) do not compare to hardware-native Unreal and Unity experiences nor offer a path for real-world asset deployments. For example, 3D experiences often require asset deployments exceeding many gigabytes, which are impossible through a web browser. Worse, _web stacks pose many blockers that publishers have little or no ability to address, such as texturing features, performance issues, or animation pain_.

***art.media.platform*** is a bridge and toolbox that allows 3D app developers to focus on their core value proposition. It offers rich support for persistent state, user interfaces, and content immersion, allowing apps to break free of web _and_ OS limitations. _Teams, organizers, artists, engineers, scientists, and ultimately consumers need better tools to richly and safely share assets_.

### Benefits
  - __Content distribution__: Choose how to deploy your projects in a way that makes the most sense for your team and budget.
  - __Flexibility__: Enjoy strategic flexibility on how you publish apps and distribute content.
  - __Asset management__: Leverage Amp's content deployment system that allows you to deploy content updates without having to deploy new App builds.
  - __Savings__: Save resources by not maintaining UIs and custom behavior for each OS, web browser, and form factor.
  - __Cash flow__: Use Amp's payment processing module to monetize your work and receive payments through your Stripe or PayFlow account.

## A Next Generation

Previous [generations](https://github.com/plan-systems/plan-go/tags) of this work went into production in 2019 to become [PLAN 3D](https://plan-systems.org/plan-technology-components/). This [architecture](https://github.com/plan-systems/design-docs) trajectory, though ambitious, is increasingly recognized as the next inevitable step in the evolution of 3D application building.

In a world where AI-assisted exploits will only worsen, our [security model](https://github.com/plan-systems/design-docs/blob/master/PLAN-Proof-of-Correctness.md) prioritizes security and privacy. It uses nested containers and offers "state-grade" protection — all while the client runtime delivers rich, native 3D experiences for businesses, organizations, and creatives.

### Spatial Web

This framework offers in-app web browsing that pairs powerfully with spatial linking. Frameworks such as [Webview](https://developer.vuplex.com/webview/overview) are just another component in the Amp client, allowing your app to have an embedded web browser out of the box. This allows URLs and web experiences to be linked spatially or from multiple map locations.

### Geo-Spatial Linking

Geographic and spatial-centric applications such as GIS, CAD, and BIM are everywhere in modern construction and real-time logistics. Amp's 3D client natively integrates [maps and locations](https://infinity-code.com/assets/online-maps), allowing you to unify location-based linking, spatially precise environments, and first-class 3D asset integration.

### Extensibility

The less obvious value of Amp is its _extensibility_. The [`amp.App`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.app.go) interface is flexible and unrestricted, allowing you to expose anything compatible with Go. This means any Go, C, C++, or any native static or dynamic module can be wrapped and push a 3D-native UX (with stock or custom assets).

### Human Accessibility

People with loss of sight, hearing, or motor skills rely on third-party peripherals and software to interact with the world. Amp integrates with most third-party input devices, such as bearing and range sensors for the visually impaired or control sticks for physical limitations.

### Tagging

Amp's [tag system](https://github.com/art-media-platform/amp.SDK/blob/main/stdlib/tag/api.tag.go) is phonetic, AI-friendly, search-friendly, and privacy-friendly. It offers powerful and flexible linking similar to how #hashtags and [wikis](https://www.wikipedia.org/) add value. We see this system as an excellent candidate to become an [IEEE](https://www.ieee.org/) standard for markup and hashing.

## Integration Overview

This repo is lightweight and dependency-free, so it can be added to your project without consequence.

At a high level:

1. Add [amp.SDK](https://github.com/art-media-platform/amp.SDK) to your Go project. If you want to expose additional functionality, implement your own [`amp.App`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.app.go).
2. Clone [amp.host](https://github.com/art-media-platform/amp.host) (not yet public) and include your `amp.App`, similar to how a library in a C project registers a static or dynamic dependency.
3. Build `amp.host` with your additions embedded within it.
4. In your Unity or Unreal app, link in `amp.host.lib` and add the Amp UX runtime support glue.
5. On startup, [`amp.Host`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.host.go) instantiates registered `amp.App` instances as needed. During runtime, `amp.host.lib` dispatches URL requests addressed to your app and are "pinned".
6. The Amp UX runtime manages the user's experience of currently pinned URLs while providing a toolbox of extendable "stock" and "skinnable" components. Pinned requests receive state updates until they are canceled.

### Points of Interest

|                                                                                                   |                                                                                                                                                                                 |
| ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [api.tag.go](https://github.com/art-media-platform/amp.SDK/blob/main/stdlib/tag/api.tag.go)    | Versatile tagging and hash scheme that is AI and search friendly                                                                                                                  |
| [api.task.go](https://github.com/art-media-platform/amp.SDK/blob/main/stdlib/task/api.task.go) | Goroutine wrapper inspired by a conventional parent-child process model                                                                                                    |
| [api.app.go](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.app.go)           | Defines how state is requested, pushed, and merged                                                                                              |
| [api.host.go](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.host.go)         | Types and interfaces that [`amp.Host`](https://github.com/art-media-platform/amp.SDK/blob/main/amp/api.host.go) implements                                                              |
