# Embedded harness icons

These bundled marks are based on [LobeHub Lobe Icons](https://github.com/lobehub/lobe-icons), from `packages/static-svg/icons/` (MIT license; see `LICENSE.lobehub`).

They are embedded into the native Go desktop binary, with a fixed 24×24 viewport for MyGo's SVG renderer. The UI uses LobeHub's color mark when available, and monochrome marks tinted to the active theme otherwise.

Not all supported harnesses have a matching LobeHub mark. Those names intentionally use a local initial-based fallback. Icon assets are static and are not downloaded at runtime.
