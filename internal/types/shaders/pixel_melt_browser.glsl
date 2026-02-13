precision mediump float;

uniform float u_time;
uniform vec2 u_resolution;
uniform sampler2D u_texture;

varying vec2 v_texCoord;

void main() {
    vec2 uv = v_texCoord;
    float frame = u_time * 24.0;
    float t = frame * 0.01;

    float strength = smoothstep(0.5, 0.6, sin(uv.x * 20.0 + t));
    
    float brightness = length(texture2D(u_texture, uv).rgb);
    uv.y -= strength * brightness * 0.1 * sin(t);

    gl_FragColor = texture2D(u_texture, uv);
}
