
(function(){
'use strict';
var theme='dark',background='#151412';
try{if(window.parent!==window){var parentRoot=window.parent.document.documentElement,parentTheme=parentRoot.getAttribute('data-theme'),parentBackground=window.parent.getComputedStyle(parentRoot).getPropertyValue('--bg-secondary').trim();theme=parentTheme==='dark'||parentTheme==='white'?parentTheme:'light';if(parentBackground)background=parentBackground;}else{theme=window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches?'dark':'white';background=theme==='dark'?'#151412':'#fff';}}catch(_error){}
var root=document.documentElement;if(theme==='light')root.removeAttribute('data-theme');else root.setAttribute('data-theme',theme);root.style.backgroundColor=background;root.style.colorScheme=theme==='dark'?'dark':'light';
try{if(window.frameElement){window.frameElement.style.backgroundColor=background;if(window.frameElement.parentElement)window.frameElement.parentElement.style.backgroundColor=background;}}catch(_error){}
})();
