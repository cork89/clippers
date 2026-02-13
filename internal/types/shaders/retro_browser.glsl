precision mediump float;

uniform float u_time;
uniform vec2 u_resolution;
uniform sampler2D u_texture;

varying vec2 v_texCoord;

float noise(vec2 p) {
    return fract(sin(dot(p, vec2(12.9898, 78.233))) * 43758.5453);
}

void main() {
    vec2 uv = v_texCoord;
    float frame = u_time * 24.0;
    float t = frame * 0.01;

    uv.x += sin(uv.y * 10.0 + t * 2.0) * 0.001;
    uv.x += sin(uv.y * 100.0 + t * 5.0) * 0.0005;

    float lineNoise = step(0.997, noise(vec2(t, uv.y)));
    uv.x += lineNoise * 0.02 * sin(t * 50.0);

    float edgeDist = length(uv - 0.5);
    float abAmount = 0.003 + (edgeDist * 0.005);
    float r = texture2D(u_texture, vec2(uv.x + abAmount, uv.y)).r;
    float g = texture2D(u_texture, uv).g;
    float b = texture2D(u_texture, vec2(uv.x - abAmount, uv.y)).b;
    
    vec3 col = vec3(r, g, b);

    float scanline = sin(uv.y * 800.0 + t) * 0.03;
    col -= scanline;

    float grain = noise(uv + t) * 0.06;
    col += grain;

    col *= 1.0 - (edgeDist * 0.5);

    col = mix(col, vec3(0.1, 0.1, 0.15), 0.05);

    gl_FragColor = vec4(col, 1.0);
}
