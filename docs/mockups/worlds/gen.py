import base64,os
R='/Users/tyler/Projects/fistbump/app/renderer/'
def b(p):
    mt='font/woff2' if p.endswith('woff2') else 'image/png'
    return 'data:%s;base64,%s'%(mt,base64.b64encode(open(R+p,'rb').read()).decode())
FONT=b('fonts/bricolage-grotesque-latin.woff2'); PICO=b('views/graphics/mascots/home/03-youve-got-this.png'); LOGO=b('views/graphics/logo.png')
jobs=[('Data Scientist, Growth','Lumen Fields','Austin, TX','Saved','81','Tailor resume'),
('Product Analyst','Fernway Health','Remote, US','Saved','74','Tailor resume'),
('Financial Analyst','Cobalt Rivers Financial','Charlotte, NC','Saved','58','Tailor resume'),
('Operations Analyst','Harbor & Pine','Portland, OR','Applied','71','Waiting · 6 days'),
('BI Developer','Northgate Energy','Houston, TX','Interviewing','77','Interview Thu 2:00 pm')]
SEARCH='<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="11" cy="11" r="7"/><path d="M20 20l-4-4"/></svg>'
GEAR='<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/></svg>'
def rows(tpl): return ''.join(tpl.format(t=t,c=c,l=l,s=s,f=f,n=n,i=c[0]) for t,c,l,s,f,n in jobs)
def page(title,css,body):
    return f'<!doctype html><html lang="en"><head><meta charset="utf-8"><title>{title}</title><style>@font-face{{font-family:Bricolage;src:url({FONT});font-weight:200 800}}*{{box-sizing:border-box;margin:0}}button{{font:inherit;color:inherit;border:0;background:none;cursor:pointer}}{css}</style></head><body>{body}</body></html>'
W={}
# A — Paper desk (light, warm)
W['a-paper']=page('A · Paper desk','''
body{background:#f3eee4;color:#1c1a2b;font:15px/1.5 system-ui,-apple-system,sans-serif;height:100vh;display:flex;flex-direction:column}
main{flex:1;padding:40px 48px 24px;display:grid;grid-template-columns:1fr 1fr;gap:28px 40px;align-content:start}
.hello{grid-column:1/-1;display:flex;align-items:flex-end;justify-content:space-between}
.hello h1{font:780 46px/1.1 Bricolage;letter-spacing:-.03em}.hello p{color:#5d5873;font-size:17px;margin-top:6px}
.lead{grid-column:1/-1;display:grid;grid-template-columns:auto 1fr auto;gap:28px;align-items:center;background:#fffdf8;border-radius:20px;padding:26px 30px;box-shadow:0 1px 0 #00000008,0 18px 40px -22px #4a361733;border:1px solid #e8e0d0}
.lead img{width:96px;margin:-30px 0 -14px}
.lead small{font-weight:600;color:#c2492f}.lead h2{font:740 28px/1.15 Bricolage;letter-spacing:-.02em;margin:2px 0}.lead span{color:#5d5873}
.go{background:#1f2a6b;color:#fff;font-weight:650;padding:13px 22px;border-radius:12px;box-shadow:0 2px 0 #10164a,0 8px 18px -8px #1f2a6b88}
.list{grid-column:1/-1;background:#fffdf8;border:1px solid #e8e0d0;border-radius:20px;padding:6px 10px}
.list h3{font:650 14px system-ui;color:#5d5873;padding:14px 14px 6px}
.r{display:grid;grid-template-columns:44px 1fr 120px 60px 200px;gap:16px;align-items:center;padding:12px 14px;border-top:1px solid #efe8da}
.r:hover{background:#f8f3e9;border-radius:12px}
.m{width:40px;height:40px;border-radius:11px;display:grid;place-items:center;font:700 17px Bricolage;background:#e9e2f6;color:#3b2f7a}
.r:nth-child(3) .m{background:#dcefe5;color:#1f5a3c}.r:nth-child(4) .m{background:#f6e2d6;color:#86391f}.r:nth-child(5) .m{background:#dde6f6;color:#22427a}.r:nth-child(6) .m{background:#f3ead0;color:#6d5410}
.r b{display:block;font-weight:620}.r .c{color:#6a6580;font-size:13px}.r .s{color:#5d5873;font-size:14px}.r .f{font-variant-numeric:tabular-nums;font-weight:600;text-align:right}.r .n{text-align:right;color:#5d5873;font-size:14px}
footer{height:60px;display:flex;align-items:center;gap:6px;padding:0 48px;border-top:1px solid #e3dac8;background:#efe9dd}
footer img{height:24px;margin-right:18px;filter:invert(1) hue-rotate(180deg) brightness(.35)}
.nav{padding:8px 14px;border-radius:10px;font-weight:600;color:#5d5873}.nav.on{background:#fffdf8;color:#1c1a2b;box-shadow:0 1px 2px #0000001a}
.q{flex:1;max-width:560px;margin:0 auto;display:flex;gap:10px;align-items:center;background:#fffdf8;border:1px solid #e3dac8;border-radius:12px;padding:9px 14px;color:#6a6580}.q kbd{margin-left:auto;font:600 11px system-ui}
''',f'''<main><div class="hello"><div><h1>Good evening, Ada.</h1><p>Wednesday, November 25 · one thing needs you</p></div></div>
<div class="lead"><img src="{PICO}" alt=""><div><small>Ready for your review</small><h2>Data Analyst at Tidewater Logistics</h2><span>2 suggested changes · 76% fit · Savannah, GA</span></div><button class="go">Review changes</button></div>
<div class="list"><h3>Your jobs</h3>{rows('<div class="r"><div class="m">{i}</div><div><b>{t}</b><span class="c">{c} · {l}</span></div><span class="s">{s}</span><span class="f">{f}%</span><span class="n">{n}</span></div>')}</div></main>
<footer><img src="{LOGO}" alt="fistbump"><button class="nav on">Today</button><button class="nav">Jobs</button><button class="nav">Resume</button><div class="q">{SEARCH}Search, paste a link, or drop in a posting<kbd>⌘K</kbd></div><button class="nav" aria-label="Settings">{GEAR}</button></footer>''')
# B — Night studio (rich dark, gold)
W['b-night']=page('B · Night studio','''
body{background:#0a1029;color:#eef0fb;font:15px/1.5 system-ui,-apple-system,sans-serif;height:100vh;display:flex;flex-direction:column}
.top{background:linear-gradient(#18215a,#111845);padding:34px 48px 0;display:flex;align-items:flex-end;gap:36px;border-bottom:1px solid #ffffff14}
.top img{width:150px;margin-bottom:-6px}
.top .t{flex:1;padding-bottom:30px}.top h1{font:760 44px/1.1 Bricolage;letter-spacing:-.03em}.top p{color:#b9c0e4;font-size:17px;margin-top:6px}
.next{background:#0a1029;border:1px solid #ffffff1c;border-radius:16px 16px 0 0;border-bottom:0;padding:20px 24px;min-width:430px;align-self:flex-end}
.next small{color:#f2c14e;font-weight:650}.next h2{font:720 22px/1.2 Bricolage;margin:4px 0 2px}.next span{color:#b9c0e4;font-size:14px}
.next .row{display:flex;justify-content:space-between;align-items:center;margin-top:14px}
.go{background:#f2c14e;color:#1b1503;font-weight:700;padding:11px 20px;border-radius:11px;box-shadow:0 8px 20px -10px #f2c14e99}
main{flex:1;padding:22px 48px;display:grid;grid-template-columns:repeat(auto-fill,minmax(380px,1fr));gap:14px;align-content:start}
.tile{background:#121a43;border-radius:16px;padding:18px 20px;display:grid;grid-template-columns:44px 1fr auto;gap:4px 14px;align-items:center;border:1px solid #ffffff0d}
.tile:hover{background:#17205200;border-color:#ffffff26}
.m{grid-row:span 2;width:44px;height:44px;border-radius:12px;display:grid;place-items:center;font:700 18px Bricolage;background:#1f2a6b;color:#cdd5ff}
.tile b{font-weight:620}.tile .c{color:#a8b0d8;font-size:13px}
.tile .f{font:700 20px Bricolage;font-variant-numeric:tabular-nums;text-align:right}.tile .s{font-size:12px;color:#a8b0d8;text-align:right}
.tile .n{grid-column:2/-1;margin-top:10px;padding-top:10px;border-top:1px solid #ffffff12;font-size:13px;color:#cfd5f2}
footer{height:58px;display:flex;align-items:center;gap:6px;padding:0 48px;border-top:1px solid #ffffff14;background:#080d22}
footer img{height:24px;margin-right:18px}
.nav{padding:8px 14px;border-radius:10px;font-weight:600;color:#a8b0d8}.nav.on{color:#fff;background:#18215a}
.q{flex:1;max-width:560px;margin:0 auto;display:flex;gap:10px;align-items:center;background:#121a43;border-radius:12px;padding:9px 14px;color:#a8b0d8}.q kbd{margin-left:auto;font:600 11px system-ui}
''',f'''<div class="top"><img src="{PICO}" alt=""><div class="t"><h1>Good evening, Ada.</h1><p>Three jobs are moving. One needs you.</p></div>
<div class="next"><small>Needs you</small><h2>Data Analyst · Tidewater Logistics</h2><span>2 suggested changes to decide</span><div class="row"><span style="font:700 26px Bricolage;color:#fff">76%<span style="font:500 13px system-ui;color:#a8b0d8"> fit</span></span><button class="go">Review changes</button></div></div></div>
<main>{rows('<div class="tile"><div class="m">{i}</div><b>{t}</b><span class="f">{f}%</span><span class="c">{c} · {l}</span><span class="s">{s}</span><span class="n">{n}</span></div>')}</main>
<footer><img src="{LOGO}" alt="fistbump"><button class="nav on">Today</button><button class="nav">Jobs</button><button class="nav">Resume</button><div class="q">{SEARCH}Search, paste a link, or drop in a posting<kbd>⌘K</kbd></div><button class="nav" aria-label="Settings">{GEAR}</button></footer>''')
# C — Color block (bold brand band)
W['c-block']=page('C · Color block','''
body{background:#f7f7fb;color:#15183a;font:15px/1.5 system-ui,-apple-system,sans-serif;height:100vh;display:flex;flex-direction:column}
.band{background:#3b4fe0;color:#fff;padding:36px 48px 64px;position:relative;overflow:hidden}
.band:after{content:"";position:absolute;right:-120px;top:-160px;width:520px;height:520px;border-radius:50%;background:#4a5ef0}
.band h1{font:800 56px/1.02 Bricolage;letter-spacing:-.035em;position:relative;z-index:1;max-width:14ch}
.band p{position:relative;z-index:1;color:#dfe3ff;font-size:17px;margin-top:10px}
.band img{position:absolute;right:90px;bottom:-34px;width:190px;z-index:2;filter:drop-shadow(0 18px 24px #0b136644)}
main{flex:1;margin-top:-34px;padding:0 48px;position:relative;z-index:1}
.sheet{background:#fff;border-radius:22px;box-shadow:0 20px 50px -28px #15183a55,0 1px 0 #15183a0d;padding:8px 10px;max-width:calc(100% - 260px)}
.lead{display:flex;align-items:center;gap:20px;padding:20px 18px;border-bottom:1px solid #ececf4}
.lead .d{width:12px;height:12px;border-radius:50%;background:#ff6a3d;box-shadow:0 0 0 5px #ff6a3d26}
.lead div{flex:1}.lead b{font:720 20px Bricolage}.lead span{display:block;color:#5c6085;font-size:14px}
.go{background:#15183a;color:#fff;font-weight:650;padding:11px 20px;border-radius:11px}
.r{display:grid;grid-template-columns:1fr 110px 54px 190px;gap:16px;align-items:center;padding:13px 18px;border-top:1px solid #f0f0f6}
.r:nth-child(2){border-top:0}.r:hover{background:#f6f7ff;border-radius:12px}
.r b{display:block;font-weight:620}.r .c{color:#6b6f92;font-size:13px}.r .s{font-size:13px;font-weight:600;color:#3b4fe0}.r .f{text-align:right;font-weight:650;font-variant-numeric:tabular-nums}.r .n{text-align:right;color:#5c6085;font-size:14px}
footer{height:58px;display:flex;align-items:center;gap:6px;padding:0 48px;border-top:1px solid #e6e7f0;background:#fff}
footer img{height:24px;margin-right:18px;filter:invert(1) hue-rotate(180deg) brightness(.3)}
.nav{padding:8px 14px;border-radius:10px;font-weight:600;color:#5c6085}.nav.on{color:#3b4fe0;background:#eef0ff}
.q{flex:1;max-width:560px;margin:0 auto;display:flex;gap:10px;align-items:center;background:#f3f4f9;border-radius:12px;padding:9px 14px;color:#6b6f92}.q kbd{margin-left:auto;font:600 11px system-ui}
''',f'''<div class="band"><h1>Good evening, Ada.</h1><p>One thing needs you. Three jobs are moving.</p><img src="{PICO}" alt=""></div>
<main><div class="sheet"><div class="lead"><span class="d"></span><div><b>Data Analyst · Tidewater Logistics</b><span>2 suggested changes to decide · 76% fit</span></div><button class="go">Review changes</button></div>
{rows('<div class="r"><div><b>{t}</b><span class="c">{c} · {l}</span></div><span class="s">{s}</span><span class="f">{f}%</span><span class="n">{n}</span></div>')}</div></main>
<footer><img src="{LOGO}" alt="fistbump"><button class="nav on">Today</button><button class="nav">Jobs</button><button class="nav">Resume</button><div class="q">{SEARCH}Search, paste a link, or drop in a posting<kbd>⌘K</kbd></div><button class="nav" aria-label="Settings">{GEAR}</button></footer>''')
for k,v in W.items(): open(k+'.html','w').write(v)
