# ModMux 应用图标

## 当前使用：开发工具风格 v3

最新设计采用蓝色几何折带 M，参考开发工具的简洁视觉语言。当前源图、macOS 图标及完整提示词见 [ICON-v3.md](ICON-v3.md)。打包脚本使用 v3；以下版本作为探索记录保留。

## 探索版本：Crystal v2

按用户要求，使用内置 imagegen 将第一版转换为 Everaldo Coelho 的 Crystal 图标风格：通透的钴蓝玻璃底板、银白金属路由标记、三个带银色边框的蓝色玻璃端点。保留原有路由结构与透明外边距。

- `ModMux-crystal-v2.png`：Crystal 探索版本原图。
- Crystal 图标的转换与打包被用户中断，当前应用采用 v3。

生成提示词：

```text
Use case: logo-brand, style-transfer edit. Asset: the final macOS application icon for ModMux, an industrial Modbus conversion and routing gateway. Edit the supplied ModMux icon into the iconic Crystal desktop icon style of Everaldo Coelho: luminous saturated cobalt and azure glass, carefully modeled glossy highlights, soft reflective gradients, beveled silver-white metal, charming tactile early-2000s premium desktop icon craftsmanship, crisp silhouette and controlled depth. Preserve the recognisable branching M routing symbol and exactly three circular connection terminals from the reference, in approximately the same geometry and placement. Replace the dark graphite matte tile with a rich luminous blue crystal/glass rounded square tile, subtly beveled, with believable translucent layered depth. Make the routing mark polished silver-white with broad clean bright faces and restrained shaded edge bevels. Make the three circular terminals glossy azure glass with clean white specular highlights and fine silver bezels. Keep the symbol very bold, clear and recognisable at 32 and 64 pixels. Upper-left soft studio illumination, restrained dimensionality, predominantly front-facing, no extreme perspective. Exactly one finished square icon, no mockup or sheet. Genuine transparent exterior beyond the tile and its small soft contact shadow; preserve comfortable transparent margins around all four sides. No words, lettering, signature, watermark, extra terminals, tiny circuitry, cables outside the tile, sparkles, lens flare, scenery or illustrative clutter. This is a usable application icon, with accurate sharp edges, refined purposeful material contrast and clear ModMux network-routing identity.
```

## 第一版：石墨色

图标以深石墨色圆角底板、白色连续路由标记和三个蓝色连接端点表达 Modbus 网关的汇聚与分流。主标记借用 M 的折线结构，不放文字或细小电路细节，适合 Dock 与应用列表。

- `ModMux.png`：imagegen 生成的原始图像，透明外边距。
- `ModMux.icns`：由原图转换的 macOS 应用图标，含 16、32、64、128、256、512、1024 像素资源。
- `Info.plist` 与打包脚本引用 `.icns`；PNG 作为源文件保留。

生成方式：内置 imagegen；下列为完整提示词。后续改稿另存版本，避免覆盖本轮源文件。

```text
Use case: logo-brand. Asset type: a finished macOS application icon for ModMux, an industrial Modbus protocol conversion and routing gateway. Generate exactly ONE polished app icon, square 1024x1024 composition, not a presentation sheet, not a mockup, no multiple variants. Actual transparent pixels outside the icon tile. Center a large graphite-black softly rounded square tile occupying about 88 percent of canvas with a restrained native macOS material finish: very subtle bevel and soft upper-left lighting, understated satin surface, no busy texture. On the tile, create a unique bold ivory-white abstract capital M monogram made from clean continuous rounded routing paths. The M must be instantly legible, geometrically coherent, balanced, with generous negative space. Its paths should subtly suggest data moving through a gateway, connecting one upstream to multiple downstreams. Integrate only two or three small vivid cool-blue circular terminal points into the ends of the mark, as meaningful connection endpoints. Large white mark occupying roughly 58 percent of canvas, strong thick strokes, suitable to read at 32px and 64px in a Dock. Mostly flat mark with very subtle dimensionality, high contrast, refined precise engineering-tool identity, credible desktop app aesthetic, beautifully clean anti-aliased silhouette. No words, no tiny typography, no slogan, no arrows, no circuit board details, no wires outside the tile, no extra decorative symbols, no gear, no robot, no globe, no photoreal objects, no watermark. Keep the whole tile and all its rounded corners visible with clear transparent margins. Deliver only the final application icon artwork.
```
