# Three.js local distribution

- Package: `three@0.170.0`
- Source: https://registry.npmjs.org/three/-/three-0.170.0.tgz
- Registry metadata: https://registry.npmjs.org/three/0.170.0
- Included unmodified: `package/build/three.module.min.js`, `package/LICENSE`.
- License: MIT, retained in `LICENSE`.
- Tarball integrity verified against registry metadata before extraction:
  `sha512-FQK+LEpYc0fBD+J8g6oSEyyNzjp+Q7Ks1C568WWaoMRLW+TkNNWmenWeGgJjV105Gd+p/2ql1ZcjYvNiPZBhuQ==`

The application imports this local module only. There are no CDN requests, remote
fonts, image models, telemetry, external textures, or network loaders in the scene.
Furniture and avatar geometry are original code in `../office-scene.js`.

Documentation consulted: https://threejs.org/docs/ (WebGLRenderer, geometry,
orthographic camera, raycasting). Runtime and performance remain unverified until
the user authorizes opening the product.
