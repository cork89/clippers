precision mediump float;

uniform float u_time;
uniform vec2 u_resolution;
uniform sampler2D u_texture;

varying vec2 v_texCoord;

// 2D -> 2D hash in [0,1)
vec2 hash22(vec2 p) {
    float n = sin(dot(p, vec2(127.1, 311.7)));
    float m = sin(dot(p, vec2(269.5, 183.3)));
    return fract(vec2(43758.5453 * n, 43758.5453 * m));
}

// Same idea, but returns a stable phase per cell (0..1)
float hash21(vec2 p) {
    return fract(sin(dot(p, vec2(12.9898, 78.233))) * 43758.5453);
}

void main() {
    vec2 uv = v_texCoord;

    // Voronoi density
    float scale = 18.0;

    // Animation controls
    float speed = 0.6;     // overall motion speed
    float jitter = 0.45;   // how far points move within their cell (0..~0.5)

    // Cell space
    vec2 p = uv * scale;
    vec2 ip = floor(p);
    vec2 fp = fract(p);

    float F1 = 1e9;
    float F2 = 1e9;
    vec2 bestCell = ip;

    // 3x3 neighbor search (Worley)
    for (int y = -1; y <= 1; y++) {
        for (int x = -1; x <= 1; x++) {
            vec2 cell = ip + vec2(float(x), float(y));

            // Base random point
            vec2 rnd = hash22(cell);

            // Per-cell phase so they don't all move the same
            float ph = hash21(cell) * 6.28318530718; // 2*pi

            // Animate the feature point with a smooth loop
            vec2 anim = vec2(
                sin(u_time * speed + ph),
                cos(u_time * speed * 1.17 + ph * 1.3)
            );

            // Feature point in [0,1) with animated offset, kept inside cell
            vec2 featureInCell = rnd + anim * jitter;
            featureInCell = clamp(featureInCell, vec2(0.05), vec2(0.95));

            // Feature point relative to ip
            vec2 feature = vec2(float(x), float(y)) + featureInCell;

            vec2 d = fp - feature;
            float dist2 = dot(d, d);

            if (dist2 < F1) {
                F2 = F1;
                F1 = dist2;
                bestCell = cell;
            } else if (dist2 < F2) {
                F2 = dist2;
            }
        }
    }

    // Sample a stable "flat" color for the winning cell
    vec2 cellUV = (bestCell + 0.5) / scale;
    cellUV = clamp(cellUV, vec2(0.0), vec2(1.0));
    vec3 col = texture2D(u_texture, cellUV).rgb;

    // Border from F2-F1
    float edge = sqrt(max(F2, 0.0)) - sqrt(max(F1, 0.0));

    float borderWidth = 0.06;
    float borderSoft = 0.02;
    float borderMask = smoothstep(borderWidth, borderWidth + borderSoft, edge);

    // Your slow shine (kept)
    float frame = u_time * 24.0;
    float t = frame * 0.008;

    vec2 lightPos = vec2(
        0.5 + 0.3 * sin(t * 0.5),
        0.5 + 0.3 * cos(t * 0.7)
    );

    float dist = distance(uv, lightPos);
    float shine = smoothstep(0.8, 0.0, dist) * 0.4;

    vec3 finalCol = (col + shine) * borderMask;

    gl_FragColor = vec4(finalCol, 1.0);
}