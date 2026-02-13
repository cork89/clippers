precision mediump float;

uniform float u_time;
uniform vec2 u_resolution;
uniform sampler2D u_texture;

varying vec2 v_texCoord;

void main() {
    vec2 uv = v_texCoord;
    float frame = u_time * 24.0;
    float t = frame * 0.008;

    vec2 warp = uv;
    for(float i = 1.0; i < 4.0; i++) {
        warp.x += 0.3 / i * sin(i * 3.0 * uv.y + t);
        warp.y += 0.3 / i * cos(i * 3.0 * uv.x + t);
    }

    vec2 distortedUV = mix(uv, warp, 0.12);
    vec4 col = texture2D(u_texture, distortedUV);

    float pattern = sin(warp.x + warp.y) * 0.5 + 0.5;
    vec3 pink = vec3(1.0, 0.75, 0.9);
    vec3 cyan = vec3(0.7, 0.95, 1.0);
    vec3 pastelTint = mix(pink, cyan, pattern);

    col.rgb = col.rgb * (pastelTint + 0.5);
    
    float glow = smoothstep(0.8, 1.0, pattern);
    col.rgb += glow * 0.15;

    gl_FragColor = col;
}
