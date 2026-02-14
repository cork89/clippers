// ./internal/views/static/settings.js

const shaderOptions = [
    { value: 'none', label: 'None', color: '#3a3a3a' },
    { value: 'wave_displace', label: 'Wave', color: '#6366f1' },
    { value: 'edge_glow', label: 'Edge', color: '#ec4899' },
    { value: 'liquid_flow', label: 'Liquid', color: '#14b8a6' },
    { value: 'pixel_melt', label: 'Pixel', color: '#f59e0b' },
    { value: 'retro', label: 'Retro', color: '#8b5cf6' },
    { value: 'voronoi', label: 'Voronoi', color: '#10b981' }
];

const aspectLabels = {
    '1x1': '1:1',
    '16x9': '16:9',
    '9x16': '9:16',
    '4x3': '4:3',
    '21x9': '21:9'
};

let selectedAspects = new Set();

function populateShaderSelect() {
    const select = document.getElementById('settings-shader');
    if (!select) return;
    
    select.innerHTML = '';
    shaderOptions.forEach(shader => {
        const option = document.createElement('option');
        option.value = shader.value;
        option.textContent = shader.label;
        select.appendChild(option);
    });
}

function updateSliderValue(slider) {
    const valueDisplay = document.getElementById('blur-value');
    if (valueDisplay) {
        valueDisplay.textContent = slider.value;
    }
}

function toggleAspectsDropdown() {
    const dropdown = document.getElementById('aspects-dropdown');
    if (dropdown) {
        dropdown.classList.toggle('show');
    }
}

function toggleAspect(value) {
    const container = document.querySelector('.multi-select-container');
    const hiddenInput = document.getElementById('settings-aspects');
    
    if (selectedAspects.has(value)) {
        selectedAspects.delete(value);
    } else {
        selectedAspects.add(value);
    }
    
    renderSelectedAspects();
    
    if (hiddenInput) {
        hiddenInput.value = Array.from(selectedAspects).join(',');
    }
    
    updateDropdownOptions();
}

function renderSelectedAspects() {
    const container = document.querySelector('.multi-select-container');
    if (!container) return;
    
    const placeholder = container.querySelector('.multi-select-placeholder');
    
    if (selectedAspects.size === 0) {
        placeholder.style.display = 'block';
        container.querySelectorAll('.multi-select-tag').forEach(tag => tag.remove());
    } else {
        placeholder.style.display = 'none';
        container.querySelectorAll('.multi-select-tag').forEach(tag => tag.remove());
        
        selectedAspects.forEach(value => {
            const tag = document.createElement('span');
            tag.className = 'multi-select-tag';
            tag.innerHTML = `
                ${aspectLabels[value] || value}
                <button class="remove-tag" onclick="event.stopPropagation(); toggleAspect('${value}')">×</button>
            `;
            container.appendChild(tag);
        });
    }
}

function updateDropdownOptions() {
    document.querySelectorAll('.multi-select-option').forEach(option => {
        const value = option.dataset.value;
        if (selectedAspects.has(value)) {
            option.classList.add('selected');
        } else {
            option.classList.remove('selected');
        }
    });
}

function initAspectsFromValue(value) {
    if (!value) {
        selectedAspects = new Set(['1x1', '16x9', '9x16']);
    } else {
        selectedAspects = new Set(value.split(',').map(s => s.trim()).filter(s => s));
    }
    renderSelectedAspects();
    updateDropdownOptions();
}

async function openSettingsModal() {
    const modal = document.getElementById('settings-modal');
    if (!modal) {
        console.error('Settings modal not found in DOM');
        return;
    }
    
    console.log('Opening settings modal');
    
    populateShaderSelect();
    
    try {
        const response = await fetch('/api/project/settings');
        if (response.ok) {
            const settings = await response.json();
            document.getElementById('settings-shader').value = settings.shader || 'none';
            document.getElementById('settings-aspects').value = settings.aspects || '1x1,16x9,9x16';
            document.getElementById('settings-font-size').value = settings.font_size || 60;
            document.getElementById('settings-subtitle-margin').value = settings.subtitle_margin || 20;
            document.getElementById('settings-blur').value = settings.blur_strength || 20;
            updateSliderValue(document.getElementById('settings-blur'));
            
            initAspectsFromValue(settings.aspects);
        }
    } catch (e) {
        console.error('Failed to load settings:', e);
        initAspectsFromValue('1x1,16x9,9x16');
    }
    
    modal.classList.remove('hidden');
    
    document.addEventListener('click', handleOutsideClick);
}

function handleOutsideClick(event) {
    const dropdown = document.getElementById('aspects-dropdown');
    const container = document.getElementById('aspects-multi-select');
    
    if (dropdown && container && !container.contains(event.target)) {
        dropdown.classList.remove('show');
    }
}

function closeSettingsModal() {
    const modal = document.getElementById('settings-modal');
    if (modal) {
        modal.classList.add('hidden');
    }
    
    document.removeEventListener('click', handleOutsideClick);
    
    const dropdown = document.getElementById('aspects-dropdown');
    if (dropdown) {
        dropdown.classList.remove('show');
    }
}

async function saveSettings() {
    const settings = {
        shader: document.getElementById('settings-shader').value,
        aspects: document.getElementById('settings-aspects').value || '1x1,16x9,9x16',
        font_size: parseInt(document.getElementById('settings-font-size').value) || 60,
        subtitle_margin: parseInt(document.getElementById('settings-subtitle-margin').value) || 20,
        blur_strength: parseInt(document.getElementById('settings-blur').value) || 20
    };
    
    try {
        const response = await fetch('/api/project/settings', {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(settings)
        });
        
        if (response.ok) {
            closeSettingsModal();
            
            const toast = document.createElement('div');
            toast.className = 'toast-notification';
            toast.textContent = 'Settings saved successfully!';
            document.body.appendChild(toast);
            setTimeout(() => toast.classList.add('show'), 10);
            setTimeout(() => {
                toast.classList.remove('show');
                setTimeout(() => toast.remove(), 300);
            }, 3000);
        } else {
            alert('Failed to save settings');
        }
    } catch (e) {
        console.error('Failed to save settings:', e);
        alert('Failed to save settings');
    }
}

async function previewShaderOnEditor(shaderName) {
    console.log('Preview shader on editor:', shaderName);
    
    const editorContent = document.getElementById('editor-panel-content');
    if (!editorContent) return;
    
    const previewImage = editorContent.querySelector('#segment-preview-image');
    if (!previewImage) return;
    
    let canvas = editorContent.querySelector('#shader-canvas');
    if (!canvas) {
        canvas = document.createElement('canvas');
        canvas.id = 'shader-canvas';
        canvas.style.display = 'none';
        canvas.style.width = '100%';
        canvas.style.borderRadius = 'var(--radius-md)';
        canvas.style.marginBottom = '1rem';
        canvas.style.boxShadow = 'var(--shadow-md)';
        
        if (previewImage.parentNode) {
            previewImage.parentNode.insertBefore(canvas, previewImage);
        }
    }
    
    if (shaderName === 'none' || !shaderName) {
        canvas.style.display = 'none';
        previewImage.style.display = 'block';
        
        if (window.shaderPreview) {
            window.shaderPreview.cleanup();
            window.shaderPreview = null;
        }
        return;
    }
    
    canvas.style.display = 'block';
    previewImage.style.display = 'none';
    
    if (!window.shaderPreview) {
        window.shaderPreview = new ShaderPreview('shader-canvas');
    }
    
    const preview = window.shaderPreview;
    
    const img = new Image();
    img.crossOrigin = 'anonymous';
    img.onload = async () => {
        preview.setImage(img);
        preview.resize();
        
        if (shaderName && shaderName !== 'none') {
            await preview.loadShader(shaderName);
            preview.start();
        }
    };
    img.src = previewImage.src;
}

document.addEventListener('DOMContentLoaded', function() {
    const modal = document.getElementById('settings-modal');
    if (modal) {
        modal.addEventListener('click', function(e) {
            if (e.target === modal) {
                closeSettingsModal();
            }
        });
    }
});
