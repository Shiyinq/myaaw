const appData = [
    {id: 'openai', name: 'ChatGPT', url: 'https://chatgpt.com/', color: '#000000', icon: 'C', iconURL: 'images/openai.png'},
    {id: 'gemini', name: 'Gemini', url: 'https://gemini.google.com/', color: '#4285F4', icon: 'G', iconURL: 'images/gemini.png'},
    {id: 'silicon', name: 'SiliconFlow', url: 'https://cloud.siliconflow.cn/playground/chat', color: '#8A2BE2', icon: 'S', iconURL: 'images/silicon.png'},
    {id: 'deepseek', name: 'DeepSeek', url: 'https://chat.deepseek.com/', color: '#1E90FF', icon: 'D', iconURL: 'images/deepseek.png'},
    {id: 'yi', name: '万知', url: 'https://www.wanzhi.com/', color: '#000000', icon: '万', iconURL: 'images/wanzhi.jpg'},
    {id: 'zhipu', name: '智谱清言', url: 'https://chatglm.cn/main/alltoolsdetail', color: '#4169E1', icon: '智', iconURL: 'images/qingyan.png'},
    {id: 'moonshot', name: 'Kimi', url: 'https://kimi.moonshot.cn/', color: '#000000', icon: 'K', iconURL: 'images/kimi.jpg'},
    {id: 'baichuan', name: '百小应', url: 'https://ying.baichuan-ai.com/chat', color: '#FFA500', icon: '百', iconURL: 'images/baixiaoying.webp'},
    {id: 'dashscope', name: '通义千问', url: 'https://tongyi.aliyun.com/qianwen/', color: '#8A2BE2', icon: '通', iconURL: 'images/qwen.png'},
    {id: 'stepfun', name: '跃问', url: 'https://yuewen.cn/chats/new', color: '#FFFF', icon: '跃', iconURL: 'images/yuewen.png'},
    {id: 'doubao', name: '豆包', url: 'https://www.doubao.com/chat/', color: '#87CEEB', icon: '豆', iconURL: 'images/doubao.png'},
    {id: 'minimax', name: '海螺', url: 'https://hailuoai.com/', color: '#006400', icon: '海', iconURL: 'images/hailuo.png'},
    {id: 'groq', name: 'Groq', url: 'https://chat.groq.com/', color: '#FF4500', icon: 'G', iconURL: 'images/groq.png'},
    {id: 'anthropic', name: 'Claude', url: 'https://claude.ai/', color: '#FF6347', icon: 'C', iconURL: 'images/claude.png'},
    {id: '360-ai-so', name: '360AI搜索', url: 'https://so.360.com/', color: '#FFD700', icon: '3', iconURL: 'images/ai-search.png'},
    {id: '360-ai-bot', name: 'AI 助手', url: 'https://bot.360.com/', color: '#FFFF', icon: 'A', iconURL: 'images/360-ai.png'},
    {id: 'baidu-ai-chat', name: '文心一言', url: 'https://yiyan.baidu.com/', color: '#FFFF', icon: '文', iconURL: 'images/baidu-ai.png'},
    {id: 'tencent-yuanbao', name: '腾讯元宝', url: 'https://yuanbao.tencent.com/chat', color: '#008000', icon: '腾', iconURL: 'images/yuanbao.png'},
    {id: 'sensetime-chat', name: '商量', url: 'https://chat.sensetime.com/wb/chat', color: '#8B4513', icon: '商', iconURL: 'images/sensetime.png'},
    {id: 'spark-desk', name: 'SparkDesk', url: 'https://xinghuo.xfyun.cn/desk', color: '#4682B4', icon: 'S', iconURL: 'images/sparkdesk.png'},
    {id: 'metaso', name: '秘塔AI搜索', url: 'https://metaso.cn/', color: '#4169E1', icon: '秘', iconURL: 'images/metaso.webp'},
    {id: 'poe', name: 'Poe', url: 'https://poe.com', color: '#9932CC', icon: 'P', iconURL: 'images/poe.webp'},
    {id: 'perplexity', name: 'perplexity', url: 'https://www.perplexity.ai/', color: '#000000', icon: 'p', iconURL: 'images/perplexity.webp'},
    {id: 'devv', name: 'DEVV_', url: 'https://devv.ai/', color: '#000000', icon: 'D', iconURL: 'images/devv.png'},
    {id: 'tiangong-ai', name: '天工AI', url: 'https://www.tiangong.cn/', color: '#FFFF', icon: '天', iconURL: 'images/tiangong.png'},
    {id: 'zhihu-zhiada', name: '知乎直答', url: 'https://zhida.zhihu.com/', color: '#FFFF', icon: '知', iconURL: 'images/zhihu.png'},
    {id: 'hugging-chat', name: 'HuggingChat', url: 'https://huggingface.co/chat/', color: '#FFFF', icon: 'H', iconURL: 'images/huggingchat.svg'},
    {id: 'Felo', name: 'Felo', url: 'https://felo.ai/', color: '#1E90FF', icon: 'F', iconURL: 'images/felo.png'},
    {id: 'bolt', name: 'bolt', url: 'https://bolt.new/', color: '#000000', icon: 'b', iconURL: 'images/bolt.svg'}
];

const appGrid = document.getElementById('appGrid');

appData.forEach(app => {
    const appElement = document.createElement('div');
    appElement.classList.add('app');

    const iconElement = document.createElement('a');
    iconElement.classList.add('app-icon');
    iconElement.style.backgroundColor = app.color;

    const imgElement = document.createElement('img');
    imgElement.src = app.iconURL ? app.iconURL : ''; 
    imgElement.alt = app.name;

    iconElement.appendChild(imgElement);
    iconElement.href = app.url;
    iconElement.target = '_blank';
    iconElement.style.textDecoration = 'none';

    const nameElement = document.createElement('a');
    nameElement.classList.add('app-name');
    nameElement.textContent = app.name;
    nameElement.href = app.url;
    nameElement.target = '_blank';
    nameElement.style.textDecoration = 'none'; 

    appElement.appendChild(iconElement);
    appElement.appendChild(nameElement);
    appGrid.appendChild(appElement);
});