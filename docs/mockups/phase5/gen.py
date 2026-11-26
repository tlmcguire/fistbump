# Builds the Phase 5 mockups on the app's real stylesheets, so they use the shipped v3 tokens and primitives.
idx=open('../../../app/renderer/index.html').read()
sprite=idx[idx.index('<svg class="icon-sprite"'):idx.index('</svg>',idx.index('<symbol id="ink-leaf"'))+6]
R='../../../app/renderer/'
CSS=''.join(f'<link rel="stylesheet" href="{R}styles/{n}.css">' for n in ['base','views','finish','themes','motion','v3'])
def ink(i,c=''): return f'<svg class="ink ink-{i} {c}" aria-hidden="true"><use href="#ink-{i}"/></svg>'
def icon(i): return f'<svg class="icon" aria-hidden="true"><use href="#i-{i}"/></svg>'
def bar(on):
    nav=''.join(f'<button class="bar-item"{" aria-current=page" if k==on else ""}>{k}</button>' for k in ['Home','Jobs','Resume'])
    return f'''<nav class="bottombar"><img src="{R}views/graphics/logo.png" class="wordmark" alt="Fistbump"><div class="bottombar-nav">{nav}</div>
<form class="omnibox">{icon("search")}<input placeholder="Search jobs, paste a link, or drop in a posting"><kbd>⌘K</kbd></form>
<div class="bottombar-end"><button class="bar-icon"{' aria-selected="true"' if on=="Settings" else ''}>{icon("settings")}</button></div></nav>'''
def page(title,css,body,on,sub=''):
    return f'''<!doctype html><html lang="en"><head><meta charset="utf-8"><title>{title}</title>{CSS}<style>
html,body{{height:100%;overflow:hidden}} .app{{height:100vh}} .scroll{{flex:1;min-height:0;overflow-y:auto}}
.app > .main{{display:flex;flex-direction:column}} .titlebar{{background:var(--canvas-2)}}
.grid{{display:grid;grid-template-columns:repeat(12,minmax(0,1fr));gap:18px;max-width:1500px;margin:0 auto;padding:12px 44px 30px;align-items:start}}
{css}</style></head><body>{sprite}<div class="app" data-route="{'jobs' if on=='Jobs' else 'x'}"><div class="titlebar"></div>{sub}
<main class="main v3">{body}</main>{bar(on)}</div></body></html>'''

# ---------------- Jobs list ----------------
JOBS=[
 ('T','Data Analyst','Tidewater Logistics','Savannah, GA · On-site','$78–92k','Tailored','76','Review 2 changes','need'),
 ('O','Backend Engineer','Oakline Software','Remote, US','$120–140k','Applied','69','Follow up today','urgent'),
 ('N','BI Developer','Northgate Energy','Houston, TX','','Interview','77','Interview Mon 2:00 pm','good'),
 ('L','Data Scientist, Growth','Lumen Fields','Austin, TX · Hybrid','$130–150k','Saved','81','Tailor resume · closes Fri',''),
 ('F','Product Analyst','Fernway Health','Remote, US','$95–110k','Saved','74','Tailor resume',''),
 ('H','Operations Analyst','Harbor & Pine','Portland, OR','$70–80k','Applied','71','Waiting · 6 days',''),
 ('C','Financial Analyst','Cobalt Rivers Financial','Charlotte, NC · On-site','$85k','Saved','58','Tailor resume',''),
]
def jrow(j,checked=False):
    t,title,co,loc,pay,stage,fit,nxt,tone=j
    nx=f'<span class="jl-next {tone}">{nxt}</span>'
    return f'''<div class="jl-row{" picked" if checked else ""}"><input type="checkbox" class="jl-check" aria-label="Select {title}"{" checked" if checked else ""}>
<span class="tile{" tile-mark" if stage=="Interview" else ""}">{t}</span>
<div class="jl-main"><b>{title}</b><span class="ink-2">{co} · {loc}</span></div>
<span class="jl-stage">{stage}</span><span class="jl-fit num">{fit}%</span><span class="jl-pay ink-2 num">{pay or "—"}</span>{nx}</div>'''
jobs_css='''
.jl-head{grid-column:1/-1;display:flex;align-items:flex-end;justify-content:space-between;gap:24px}
.jl-head h1{font-size:44px}
.jl-sum{margin-top:10px;color:var(--on-canvas-2);font-size:16px}.jl-sum b{color:var(--on-canvas)}
.jl-tools{grid-column:1/-1;display:flex;flex-wrap:wrap;align-items:center;gap:10px}
.jl-stages{display:inline-flex;gap:2px;padding:3px;border-radius:12px;background:color-mix(in srgb,var(--on-canvas) 9%,transparent)}
.jl-stages button{height:34px;padding:0 14px;border-radius:9px;background:transparent;color:var(--on-canvas-2);font-weight:650;font-size:13.5px}
.jl-stages button span{margin-left:6px;opacity:.75;font-variant-numeric:tabular-nums}
.jl-stages button.on{background:var(--paper);color:var(--ink)}
.jl-quick{display:flex;gap:6px}
.jl-quick button{height:34px;padding:0 12px;border-radius:999px;background:transparent;border:1.5px dashed color-mix(in srgb,var(--on-canvas) 30%,transparent);color:var(--on-canvas-2);font-size:13px;font-weight:600}
.jl-quick button.on{border-style:solid;border-color:var(--mark);color:var(--on-canvas)}
.jl-search{margin-left:auto;display:flex;align-items:center;gap:8px;height:36px;padding:0 12px;border-radius:10px;background:color-mix(in srgb,var(--on-canvas) 9%,transparent);color:var(--on-canvas-2);font-size:13.5px;min-width:180px}
.jl-search .icon{width:15px;height:15px}
.jl-view{display:inline-flex;gap:2px;padding:3px;border-radius:10px;background:color-mix(in srgb,var(--on-canvas) 9%,transparent)}
.jl-view button{height:30px;padding:0 12px;border-radius:7px;background:transparent;color:var(--on-canvas-2);font-size:13px;font-weight:650}
.jl-view button.on{background:color-mix(in srgb,var(--on-canvas) 16%,transparent);color:var(--on-canvas)}
.jl-list{grid-column:1/-1;padding:8px 22px 14px}
.jl-group{display:flex;align-items:baseline;justify-content:space-between;padding:12px 0 6px}
.jl-group h2{font-size:15px;font-weight:720}
.jl-bulk{display:flex;align-items:center;gap:6px;font-size:13px}
.jl-bulk b{margin-right:6px}
.jl-bulk .btn{min-height:30px;padding:5px 10px;font-size:13px}
.jl-row{display:grid;grid-template-columns:22px 38px minmax(0,1fr) 92px 54px 92px 210px;gap:14px;align-items:center;padding:10px 6px;border-top:1.5px dashed var(--rule)}
.jl-row:hover{background:color-mix(in srgb,var(--ink) 4%,transparent)}
.jl-row.picked{background:color-mix(in srgb,var(--mark) 14%,transparent)}
.jl-check{width:17px;height:17px;accent-color:var(--ink)}
.jl-main b{display:block;font-weight:650}.jl-main span{font-size:13px}
.jl-stage{font-size:13px;font-weight:650;color:var(--ink-2)}
.jl-fit{font:780 17px var(--display);text-align:right}
.jl-pay{font-size:13px;text-align:right}
.jl-next{justify-self:end;font-size:13px;font-weight:650;padding:5px 10px;border-radius:8px;background:var(--paper-2)}
.jl-next.need{background:color-mix(in srgb,var(--mark) 28%,var(--paper));color:var(--mark-ink)}
.jl-next.urgent{background:color-mix(in srgb,var(--urgent) 13%,var(--paper));color:var(--urgent-ink)}
.jl-next.good{background:color-mix(in srgb,var(--success) 18%,var(--paper));color:var(--success-ink)}
.jl-foot{display:flex;justify-content:space-between;margin-top:10px;padding-top:10px;border-top:1.5px dashed var(--rule);font-size:13px;color:var(--ink-2)}
.jl-foot a{color:var(--ink);font-weight:600}
'''
jobs_body=f'''<div class="scroll"><div class="grid">
<header class="jl-head"><div><h1 class="display-xl">Your <span class="underlined">jobs{ink("underline")}</span></h1>
<p class="jl-sum"><b>7 active</b> · 2 need you today · 1 interview this week · 3 applications this month</p></div></header>
<div class="jl-tools">
 <div class="jl-stages" role="tablist"><button class="on">All<span>7</span></button><button>Saved<span>3</span></button><button>Tailored<span>1</span></button><button>Applied<span>2</span></button><button>Interview<span>1</span></button><button>Offer<span>0</span></button><button>Closed<span>4</span></button></div>
 <div class="jl-quick"><button class="on">Remote</button><button>Has pay</button><button>70%+ fit</button></div>
 <div class="jl-search">{icon("search")}Filter</div>
 <div class="jl-view"><button class="on">List</button><button>Board</button></div>
</div>
<section class="paper jl-list">
 <div class="jl-group"><h2>Needs you</h2><span class="aside ink-2">Sorted by what is due first</span></div>
 {jrow(JOBS[0])}{jrow(JOBS[1])}
 <div class="jl-group"><h2>Everything else</h2>
  <div class="jl-bulk"><b>2 selected</b><button class="btn">Move to stage ▾</button><button class="btn">Archive</button><button class="btn job3-danger">Trash</button><button class="btn btn-text">Clear</button></div></div>
 {jrow(JOBS[2])}{jrow(JOBS[3],True)}{jrow(JOBS[4],True)}{jrow(JOBS[5])}{jrow(JOBS[6])}
 <div class="jl-foot"><span>Archived jobs and the Trash are under Closed.</span><span><a href="#">Export CSV</a> · <a href="#">Compare offers</a></span></div>
</section></div></div>'''
open('jobs-v3.html','w').write(page('Jobs list',jobs_css,jobs_body,'Jobs'))

# ---------------- Resume ----------------
res_css='''
.rs-sheet{grid-column:1/9;padding:38px 44px 40px;border-radius:8px;position:relative}
.rs-sheet h2.name{font-size:30px;font-weight:800;line-height:1.1}
.rs-contact{margin-top:6px;font-size:13.5px;color:var(--ink-2)}
.rs-sec{margin-top:20px}
.rs-sec-h{display:flex;align-items:center;justify-content:space-between;padding-bottom:4px;border-bottom:1.5px solid var(--rule)}
.rs-sec-h h3{font:720 12px system-ui;letter-spacing:.12em;text-transform:uppercase;color:var(--ink-2)}
.rs-sec-h .btn{min-height:28px;padding:4px 10px;font-size:12.5px}
.rs-text{font-size:14.5px;line-height:1.6;color:color-mix(in oklab,var(--ink) 90%,var(--paper));margin-top:8px}
.rs-role{display:flex;justify-content:space-between;margin-top:12px;font-weight:650}
.rs-role span{font-weight:500;color:var(--ink-2)}
.rs-ul{margin:4px 0 0;padding-left:18px;font-size:14.5px;line-height:1.6}
.rs-gap{display:inline-flex;align-items:center;gap:6px;margin-top:6px;padding:4px 10px;border-radius:8px;background:color-mix(in srgb,var(--mark) 24%,var(--paper));color:var(--mark-ink);font-size:13px;font-weight:650}
.rs-edit{margin-top:10px;padding:14px;border-radius:14px;background:var(--paper-2)}
.rs-edit label{display:block;font-size:12.5px;font-weight:650;color:var(--ink-2);margin:8px 0 4px}
.rs-edit .f{height:38px;border-radius:10px;border:1.5px solid var(--rule);background:var(--paper);padding:8px 10px;font-size:14px;color:var(--ink)}
.rs-edit .f.area{height:76px}
.rs-edit .row{display:grid;grid-template-columns:1fr 1fr;gap:10px}
.rs-edit .acts{display:flex;gap:8px;margin-top:12px}
.rs-side{grid-column:9/13;display:flex;flex-direction:column;gap:18px}
.rs-ready .big{display:flex;align-items:baseline;gap:8px}
.rs-ready .big b{font:800 40px/1 var(--display)}
.rs-ready .big span{color:var(--ink-2);font-weight:650}
.rs-check{display:grid;grid-template-columns:24px 1fr auto;gap:10px;align-items:center;padding:8px 0;border-top:1.5px dashed var(--rule);font-size:14px}
.rs-check .ink{width:22px;height:22px}
.rs-check .ink-check{color:var(--success-ink)}.rs-check .ink-open{color:var(--urgent-ink)}
.rs-check .btn{min-height:28px;padding:4px 10px;font-size:12.5px}
.rs-pico{display:flex;gap:10px;align-items:center;margin-top:10px;padding-top:10px;border-top:1.5px dashed var(--rule);font-size:13px;color:var(--ink-2)}
.rs-pico img{width:54px}
.rs-v{display:grid;grid-template-columns:1fr auto;gap:8px;align-items:center;padding:9px 0;border-top:1.5px dashed var(--rule)}
.rs-v b{display:block;font-weight:650}.rs-v span{font-size:12.5px}
.rs-v .btn{min-height:28px;padding:4px 10px;font-size:12.5px}
.rs-styles{display:flex;gap:6px;margin:6px 0 12px}
.rs-styles button{height:32px;padding:0 12px;border-radius:999px;background:var(--paper-2);color:var(--ink);font-weight:650;font-size:13px}
.rs-styles button.on{background:var(--ink);color:var(--paper)}
.rs-pdf-row{display:flex;align-items:center;justify-content:space-between;gap:8px}
'''
res_body=f'''<div class="scroll"><div class="grid">
<article class="paper rs-sheet" aria-label="Master resume">
 <h2 class="name display">Ada Okafor</h2><p class="rs-contact">Savannah, GA · ada.okafor@example.com · linkedin.com/in/adaokafor</p>
 <div class="rs-sec"><div class="rs-sec-h"><h3>Summary</h3><button class="btn btn-text">{icon("pencil")} Edit</button></div>
  <p class="rs-text">Analyst with five years of experience turning messy data into decisions.</p></div>
 <div class="rs-sec"><div class="rs-sec-h"><h3>Experience</h3><button class="btn btn-text">+ Add a role</button></div>
  <div class="rs-role">Research Assistant, Coastal University<span>2022 – now</span></div>
  <ul class="rs-ul"><li>Built 3 Tableau dashboards tracking enrollment for the lab’s studies</li><li>Cleaned and joined 40k-row survey datasets in SQL and Python</li><li>Presented weekly results to the lab’s PI</li></ul>
  <span class="rs-gap">{ink("open")} Add the impact: what changed because of this work?</span>
  <div class="rs-role">Data Analyst Intern, Harbor Freight Co.<span>2020 – 2022</span></div>
  <div class="rs-edit"><div class="row"><div><label>Title</label><div class="f">Data Analyst Intern</div></div><div><label>Company</label><div class="f">Harbor Freight Co.</div></div></div>
   <label>What you did</label><div class="f area">Automated a weekly Excel pivot report used by 12 dispatchers</div>
   <label>Skills used</label><div class="f">excel, sql</div>
   <div class="acts"><button class="btn btn-primary">Save role</button><button class="btn">Cancel</button><button class="btn btn-text job3-danger" style="margin-left:auto">Remove role</button></div></div></div>
 <div class="rs-sec"><div class="rs-sec-h"><h3>Education</h3><button class="btn btn-text">+ Add a school</button></div>
  <div class="rs-role">B.S. Statistics, Coastal University<span>2020</span></div></div>
 <div class="rs-sec"><div class="rs-sec-h"><h3>Skills</h3><button class="btn btn-text">{icon("pencil")} Edit</button></div>
  <p class="rs-text">SQL, Excel, Tableau, Python, R, statistics</p></div>
</article>
<div class="rs-side">
 <section class="paper rs-ready"><div class="paper-head"><h2>Ready to tailor?</h2></div>
  <div class="big"><b class="num">92%</b><span>complete</span></div>
  <div class="rs-check">{ink("check")}<span>Name and contact</span></div>
  <div class="rs-check">{ink("check")}<span>Summary</span></div>
  <div class="rs-check">{ink("check")}<span>2 roles, 1 school</span></div>
  <div class="rs-check">{ink("open")}<span>Impact for Research Assistant</span><button class="btn">Add</button></div>
  <div class="rs-pico"><img src="{R}views/graphics/mascots/home/03-youve-got-this.png" alt=""><span>One thing left. Numbers make an impact line land: hours saved, people helped.</span></div></section>
 <section class="paper"><div class="paper-head"><h2>PDF</h2><span class="tag tag-good">Fits on 1 page</span></div>
  <div class="rs-styles"><button class="on">Classic</button><button>Modern</button><button>Compact</button></div>
  <div class="rs-pdf-row"><button class="btn">Preview</button><button class="btn btn-primary">Export PDF</button></div></section>
 <section class="paper"><div class="paper-head"><h2>Saved versions</h2><a href="#">Save this version</a></div>
  <div class="rs-v"><div><b>Analyst, Nov</b><span class="ink-2">For Data Analyst roles · Nov 18</span></div><button class="btn btn-text">Open</button></div>
  <div class="rs-v"><div><b>Before the refresh</b><span class="ink-2">Oct 2</span></div><button class="btn btn-text">Open</button></div></section>
</div></div></div>'''
open('resume-v3.html','w').write(page('Resume',res_css,res_body,'Resume'))

# ---------------- Settings ----------------
THEMES=[('Classic','#111851'),('Bubblegum','#b8146b'),('Techno','#090d1f'),('Cyber','#040b06'),('Retro','#1b48a1'),('Vaporwave','#1c0c36'),('Midnight Violet','#21222c'),('Matcha Latte','#18261e'),('Art Deco Gold','#0e0f14'),('Ocean Abyss','#051829'),('8-Bit Arcade','#120d31'),('Seasonal (auto)','linear-gradient(135deg,#0c1f38,#38121f,#0a291c)'),
('January','#0c1f38'),('February','#360d24'),('March','#0a2b1e'),('April','#261838'),('May','#113026'),('June','#083344'),('July','#111c4e'),('August','#3b1230'),('September','#122b36'),('October','#1c1030'),('November','#38121f'),('December','#0a291c')]
sw=''.join(f'<button class="st-sw{" on" if n=="Classic" else ""}" title="{n}"><span style="background:{c}"><i></i></span><b>{n}</b></button>' for n,c in THEMES)
set_css='''
.st-head{grid-column:1/-1}
.st-head h1{font-size:44px}
.st-jump{display:flex;flex-wrap:wrap;gap:6px;margin-top:14px}
.st-jump a{padding:6px 12px;border-radius:999px;background:color-mix(in srgb,var(--on-canvas) 9%,transparent);color:var(--on-canvas-2);font-size:13px;font-weight:650;text-decoration:none}
.st-col{display:flex;flex-direction:column;gap:18px}
.st-a{grid-column:1/8}.st-b{grid-column:8/13}
.st-sws{display:grid;grid-template-columns:repeat(6,minmax(0,1fr));gap:10px 8px}
.st-sw{display:flex;flex-direction:column;align-items:center;gap:6px;padding:0;background:none;height:auto;min-height:0}
.st-sw span{position:relative;display:block;width:100%;aspect-ratio:1.5;border-radius:10px;box-shadow:inset 0 0 0 1px rgba(0,0,0,.15)}
.st-sw span i{position:absolute;left:18%;right:18%;bottom:18%;height:30%;border-radius:4px;background:#fff8ea;opacity:.9}
.st-sw b{font-size:11.5px;font-weight:600;color:var(--ink-2);text-align:center;line-height:1.2}
.st-sw.on b{color:var(--ink)}
.st-sw.on span{outline:3px solid var(--mark);outline-offset:3px}
.st-row{display:grid;grid-template-columns:1fr auto;gap:14px;align-items:center;padding:11px 0;border-top:1.5px dashed var(--rule)}
.st-row b{display:block;font-weight:650;font-size:14px}.st-row span.ink-2{font-size:13px}
.st-seg{display:inline-flex;gap:2px;padding:3px;border-radius:10px;background:var(--paper-2)}
.st-seg button{height:28px;padding:0 10px;border-radius:7px;background:transparent;color:var(--ink-2);font-size:12.5px;font-weight:650}
.st-seg button.on{background:var(--ink);color:var(--paper)}
.st-tog{width:42px;height:24px;border-radius:999px;background:color-mix(in srgb,var(--ink) 22%,transparent);position:relative}
.st-tog::after{content:"";position:absolute;top:3px;left:3px;width:18px;height:18px;border-radius:50%;background:var(--paper)}
.st-tog.on{background:var(--success-ink)}.st-tog.on::after{left:21px}
.st-engine{display:grid}
.st-eng{display:grid;grid-template-columns:24px 1fr auto;gap:10px;align-items:start;padding:12px 0;border-top:1.5px dashed var(--rule)}
.st-eng.on b{color:var(--success-ink)}.st-eng.on .icon{color:var(--success-ink)}
.st-eng .icon{width:20px;height:20px;margin-top:2px}
.st-eng b{display:block;font-weight:700}.st-eng span{font-size:13px;color:var(--ink-2)}
.st-eng .btn{min-height:30px;padding:5px 10px;font-size:12.5px}
.st-store{display:flex;height:14px;border-radius:7px;overflow:hidden;margin:10px 0 8px;background:var(--paper-2)}
.st-store i{display:block}
.st-legend{display:flex;flex-wrap:wrap;gap:12px;font-size:12.5px;color:var(--ink-2)}
.st-legend span::before{content:"";display:inline-block;width:9px;height:9px;border-radius:3px;margin-right:6px;background:var(--c)}
.st-acts{display:flex;flex-wrap:wrap;gap:8px;margin-top:12px}
.st-acts .btn{min-height:32px;padding:6px 12px;font-size:13px}
'''
set_body=f'''<div class="scroll"><div class="grid">
<header class="st-head"><h1 class="display-xl"><span class="underlined">Settings{ink("underline")}</span></h1>
<nav class="st-jump"><a href="#">Look</a><a href="#">Accessibility</a><a href="#">AI and privacy</a><a href="#">Job search</a><a href="#">Your data</a><a href="#">Updates</a><a href="#">Shortcuts</a></nav></header>
<div class="st-col st-a">
 <section class="paper"><div class="paper-head"><h2>Look</h2><span class="aside">11 styles, 12 months</span></div><div class="st-sws">{sw}</div></section>
 <section class="paper"><div class="paper-head"><h2>Accessibility</h2></div>
  <div class="st-row"><div><b>Text size</b><span class="ink-2">Everything in the window, larger or smaller</span></div><div class="st-seg"><button>Small</button><button class="on">Default</button><button>Large</button><button>Larger</button></div></div>
  <div class="st-row"><div><b>Reduce motion</b><span class="ink-2">Turns off transitions and the counting numbers</span></div><div class="st-seg"><button class="on">Match system</button><button>On</button><button>Off</button></div></div>
  <div class="st-row"><div><b>Increase contrast</b><span class="ink-2">White paper, black ink, stronger lines</span></div><span class="st-tog"></span></div>
  <div class="st-row"><div><b>Underline links</b><span class="ink-2">Every link underlined, not only on hover</span></div><span class="st-tog on"></span></div>
  <div class="st-row"><div><b>Language</b><span class="ink-2">Menus and explanations. Your resume stays as written.</span></div><div class="st-seg"><button class="on">System</button><button>English</button><button>Español</button><button>Français</button><button>Deutsch</button></div></div></section>
</div>
<div class="st-col st-b">
 <section class="paper"><div class="paper-head"><h2>AI and privacy</h2><span class="tag tag-good">Nothing leaves this computer</span></div>
  <div class="st-engine">
   <div class="st-eng on">{icon("shield")}<div><b>Built-in rules</b><span>No AI. Rewords what is already on your resume. Instant.</span></div><span class="tag tag-good">In use</span></div>
   <div class="st-eng">{icon("shield")}<div><b>Local model</b><span>Runs on this computer. Its size and memory needs are shown before it downloads.</span></div><button class="btn">Download</button></div>
   <div class="st-eng">{icon("cloud")}<div><b>Your own provider</b><span>Opt-in. Your resume text goes to the provider you set up, with your key.</span></div><button class="btn">Set up</button></div>
  </div></section>
 <section class="paper"><div class="paper-head"><h2>Your data</h2><a href="#">Show folder</a></div>
  <div class="st-store"><i style="width:8%;background:var(--ink)"></i><i style="width:3%;background:var(--mark)"></i><i style="width:1%;background:var(--success-ink)"></i></div>
  <div class="st-legend"><span style="--c:var(--ink)">Database 12 MB</span><span style="--c:var(--mark)">Search cache 4 MB</span><span style="--c:var(--success-ink)">Logos 1 MB</span><span style="--c:var(--paper-2)">Free: plenty</span></div>
  <div class="st-acts"><button class="btn">Export backup</button><button class="btn">Restore backup</button><button class="btn">Clean up old searches</button><button class="btn btn-text">Clear history</button></div></section>
 <section class="paper"><div class="paper-head"><h2>Updates</h2><span class="aside">Version 0.5.0</span></div>
  <div class="st-row"><div><b>You're up to date</b><span class="ink-2">Checked today at 9:12 am</span></div><button class="btn">Check again</button></div></section>
</div></div></div>'''
open('settings-v3.html','w').write(page('Settings',set_css,set_body,'Settings'))
