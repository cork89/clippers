//!HOOK MAIN
//!BIND HOOKED
//!DESC Animated Voronoi Mosaic (Worley) with Soft Slow Shine

// 2D -> 2D hash in [0,1)
vec2 hash22(vec2 p) {
    float n = sin(dot(p, vec2(127.1, 311.7)));
    float m = sin(dot(p, vec2(269.5, 183.3)));
    return fract(vec2(43758.5453 * n, 43758.5453 * m));
}

// 2D -> 1D hash in [0,1)
float hash21(vec2 p) {
    return fract(sin(dot(p, vec2(12.9898, 78.233))) * 43758.5453);
}

vec4 hook() {
    vec2 uv = HOOKED_pos;

    // Voronoi density (higher = smaller cells)
    float scale = 18.0;

    // Animation controls
    float speed = 0.6;    // overall motion speed
    float jitter = 0.45;  // how far points move within their cell

    // Cell space
    vec2 p = uv * scale;
    vec2 ip = floor(p);
    vec2 fp = fract(p);

    float F1 = 1e9;
    float F2 = 1e9;
    vec2 bestCell = ip;

    // 3x3 neighborhood search (Worley)
    for (int y = -1; y <= 1; y++) {
        for (int x = -1; x <= 1; x++) {
            vec2 cell = ip + vec2(float(x), float(y));

            vec2 rnd = hash22(cell);
            float ph = hash21(cell) * 6.28318530718; // 2*pi

            vec2 anim = vec2(
                sin(float(frame) * 0.01 * speed + ph),
                cos(float(frame) * 0.01 * speed * 1.17 + ph * 1.3)
            );

            vec2 featureInCell = rnd + anim * jitter;
            featureInCell = clamp(featureInCell, vec2(0.05), vec2(0.95));

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

    // Flat color per region by sampling at winning cell center
    vec2 cellUV = (bestCell + 0.5) / scale;
    cellUV = clamp(cellUV, vec2(0.0), vec2(1.0));
    vec3 col = HOOKED_tex(cellUV).rgb;

    // Border from F2-F1 (smaller => closer to edge)
    float edge = sqrt(max(F2, 0.0)) - sqrt(max(F1, 0.0));

    float borderWidth = 0.06;
    float borderSoft = 0.02;
    float borderMask = smoothstep(borderWidth, borderWidth + borderSoft, edge);

    // Your slow, soft shine
    float t = float(frame) * 0.008;
    vec2 lightPos = vec2(
        0.5 + 0.3 * sin(t * 0.5),
        0.5 + 0.3 * cos(t * 0.7)
    );

    float dist = distance(uv, lightPos);
    float shine = smoothstep(0.8, 0.0, dist) * 0.4;

    vec3 finalCol = (col + shine) * borderMask;
    return vec4(finalCol, 1.0);
}