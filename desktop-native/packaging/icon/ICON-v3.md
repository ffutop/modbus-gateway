# ModMux 开发工具风格图标 v3

按用户最新要求，以 VS Code 等开发工具的简洁几何语言重新设计。蓝色交叠折带构成 M，中央交汇表达网关汇聚与分流；不用玻璃底板、金属高光、圆形端点或文字，外侧透明。

- 源图：`ModMux-devtools-v3.png`（本目录），三个平台的图标都由它派生。
- macOS：`../macos/ModMux-devtools-v3.icns`，含普通及 Retina 尺寸，打包时在应用资源中命名为 `ModMux.icns`。
- Windows：`package-windows.sh` 构建时用 go-winres 从源图生成 256/64/48/32/16 像素的图标资源（ID 1），连同版本信息嵌入 exe。
- Linux：`../linux/icons/modmux-<尺寸>.png`（16–512 像素），打包时放入 `share/icons/hicolor`。更换源图后在 macOS 上重新生成：`for s in 16 24 32 48 64 128 256 512; do sips -s format png -z $s $s ModMux-devtools-v3.png --out ../linux/icons/modmux-$s.png; done`。
- 生成方式：内置 imagegen；之前版本保留作为探索记录。

完整提示词：

```text
Use case: logo-brand. Asset: a redesigned ModMux desktop application icon inspired by the clean geometric visual language of modern developer tools such as VS Code, while being an ORIGINAL distinct identity, not the VS Code logo. Generate exactly ONE final icon on a genuinely transparent background, no icon board, no mockup. ModMux is a Modbus protocol conversion and routing gateway. Subject: one bold angular M-like routing monogram formed from two broad interlocking folded geometric ribbons. Make the M recognisable, balanced and coherent, with a central junction subtly implying one upstream splitting into downstream paths. Crisp engineered geometry, generous negative space, confident thick ribbon faces, clean angular cuts with very subtle corner rounding. Use a restrained two-tone developer-tool blue palette: rich cobalt-blue for one face and bright azure for the other, a single darker fold face is allowed to clarify construction. Mostly flat vector-like graphic, precisely controlled shapes, at most very subtle tonal shading. A distinctive freestanding logo silhouette, no enclosing square tile, no background plaque. Center the mark within a square canvas with roughly 14 percent transparent clear margin around it. Clear and memorable at 16, 32 and 64 pixels, no narrow gaps or hairline details. No metallic material, chrome, glass, specular highlights, gloss, reflections, bevel effects, 3D rendering, contact shadow, neon glow, circular terminals, tiny circuitry, literal hardware, arrows, text, letters written separately, words, signature, watermark or decorative objects. Only the finished blue geometric ModMux routing monogram on transparent pixels. Professional, calm, modern software-tool branding.
```
