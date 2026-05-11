// Mock data for debugging. Used as fallback when /api/ideas is unavailable.
const MOCK_IDEAS = [
    {
        id: 1,
        subs: ["r/freelance", "r/digitalnomad", "r/webdev"],
        postsFound: 847,
        sampleUpvotes: 847,
        sampleComments: 203,
        samplePost:
            "Just spent 3 hours this week sending awkward reminder emails for invoices 30 days overdue. My clients are great but I feel terrible asking for my own money. There has to be a better way.",
        title: "Freelancers hate chasing invoices",
        summary:
            "Lose hours every week on awkward payment reminders. Every existing tool costs $50/mo or sends robotic emails that feel passive-aggressive.",
        problem:
            "Late payments hurt freelance cash flow. The social awkwardness of asking for money makes freelancers wait too long, which makes it worse. FreshBooks costs $50/mo and is overkill for a solo freelancer. Most just send a manual email and feel bad about it.",
        targetUser:
            "Solo freelancers earning $2k–$15k/month — mostly designers, developers, copywriters. Typically 1-2 years in, past the beginner stage.",
        solution:
            "A focused tool that auto-sends warm, personalized follow-up emails at day 7, day 14, and day 30 after invoice due date. Tone picker: friendly / firm / final notice. Sends from your own email. One-time setup per client. Nothing else.",
        monetization:
            "$9/mo flat. 60k+ freelancers on r/freelance alone. At 1% conversion that's $5,400 MRR from one subreddit. Upsell: contract templates at $19/mo.",
        competitors:
            "FreshBooks ($50/mo, overkill), Wave (free but no follow-up automation), HoneyBook ($39/mo, full CRM — too much).",
        scores: {
            marketSize: 22,
            painIntensity: 23,
            solutionGap: 20,
            monetization: 19,
        },
        total: 84,
        verdictType: "build",
        verdictLabel: "Build it",
    },
    {
        id: 2,
        subs: ["r/ADHD", "r/productivity", "r/selfimprovement"],
        postsFound: 1240,
        sampleUpvotes: 1240,
        sampleComments: 412,
        samplePost:
            "I have 47 browser tabs open right now. 23 of them are articles I need to read later. I know I will never read them. Why can I not just close them.",
        title: "People can't manage their 'read later' pile",
        summary:
            "Pocket has 40M users. Most never open saved articles again. The save behavior is compulsive. The reading never happens.",
        problem:
            "Saving content feels productive. Reading it doesn't happen. Pocket and Instapaper solved saving. Nobody solved the follow-through. The guilt of an unread list makes people save less over time.",
        targetUser:
            "Knowledge workers aged 25-40, particularly those with ADHD. Saves 5-15 articles a week. Opens their read-later list maybe once a month.",
        solution:
            "A read-later app that surfaces one article per day based on what you've actually finished reading. No list. No queue. One card, chosen for you, at a time you pick.",
        monetization:
            "$5/mo consumer. Potential B2B angle: corporate learning platforms licensing the daily delivery model. 40M Pocket users is the addressable market.",
        competitors:
            "Pocket (40M users, no delivery model), Instapaper (acquired, stagnant), Matter (newsletter-focused).",
        scores: {
            marketSize: 24,
            painIntensity: 21,
            solutionGap: 18,
            monetization: 15,
        },
        total: 78,
        verdictType: "build",
        verdictLabel: "Build it",
    },
    {
        id: 3,
        subs: ["r/smallbusiness", "r/entrepreneur", "r/marketing"],
        postsFound: 312,
        sampleUpvotes: 312,
        sampleComments: 67,
        samplePost:
            "I run a 3-person agency. Clients ask for reports every month. I spend 6 hours building the same PowerPoint. Tools exist but none of them connect to everything I use.",
        title: "Agency reporting is still mostly manual",
        summary:
            "Small agencies spend 5-8 hours a month building client reports by hand. The tools exist but they don't talk to each other, so someone still exports CSVs.",
        problem:
            "Reporting is grunt work that falls on whoever is most technical. Data lives in GA4, Meta Ads, and Ahrefs. Getting it into one coherent PDF takes half a day. Every month.",
        targetUser:
            "Agencies with 2-10 people billing $3k–$20k/month per client. Usually the founder or ops person handles reporting.",
        solution:
            "Connect the 5 most-used agency tools (GA4, Meta Ads, Google Ads, Ahrefs, Mailchimp). Generate a client-branded PDF in 2 clicks. Scheduled monthly.",
        monetization:
            "$49/mo per agency. Crowded space but nobody owns the small-agency tier below $100/mo.",
        competitors:
            "AgencyAnalytics ($12/client/mo), Databox ($88/mo), Looker Studio (free but steep learning curve).",
        scores: {
            marketSize: 15,
            painIntensity: 17,
            solutionGap: 14,
            monetization: 18,
        },
        total: 64,
        verdictType: "maybe",
        verdictLabel: "Validate first",
    },
    {
        id: 4,
        subs: ["r/sysadmin", "r/devops", "r/aws"],
        postsFound: 198,
        sampleUpvotes: 198,
        sampleComments: 89,
        samplePost:
            "Every month I get a surprise AWS bill. I set up billing alerts. They're set wrong. I forget. Then $400 disappears and I have to explain it to my boss.",
        title: "Cloud billing surprises are still a thing",
        summary:
            "AWS billing is genuinely confusing. Even experienced engineers get hit. The tools for this are all enterprise-priced or require DevOps setup to configure.",
        problem:
            "Cloud costs are opaque. AWS has 200+ services with different pricing models. Setting up alerts correctly requires understanding what you're looking for. Most teams configure them after the first bad month.",
        targetUser:
            "Solo devs and small teams (2-10 engineers) running AWS, GCP, or Azure. Spending $200–$2k/month. No dedicated DevOps.",
        solution:
            "Connect once with a read-only IAM role. Get a plain-English breakdown of what's costing what. One alert when anything spikes more than 20%.",
        monetization:
            "$19/mo solo, $49/mo team. Finance teams hate surprise cloud bills as much as engineers do — that's your B2B angle.",
        competitors:
            "CloudHealth (enterprise-only), Spot.io (complex), AWS Cost Explorer (built-in but terrible UX).",
        scores: {
            marketSize: 14,
            painIntensity: 16,
            solutionGap: 15,
            monetization: 17,
        },
        total: 62,
        verdictType: "maybe",
        verdictLabel: "Validate first",
    },
    {
        id: 5,
        subs: ["r/webdev", "r/learnprogramming", "r/cscareerquestions"],
        postsFound: 445,
        sampleUpvotes: 445,
        sampleComments: 178,
        samplePost:
            "I made a portfolio website. I have been working on it for 3 months. I have not sent it to anyone. I keep changing things.",
        title: "Dev portfolios nobody looks at",
        summary:
            "Millions of developers build portfolio sites. The problem isn't the portfolio — hiring managers don't look at them anyway.",
        problem:
            "Developers spend 50-100 hours building portfolios. Hiring managers spend 30 seconds on them. The portfolio itself isn't the bottleneck. It's the rest of the job search process.",
        targetUser:
            "Junior to mid-level developers, 1-3 YOE, actively job searching.",
        solution:
            "Unclear. The frustration is real but the solution isn't obviously a SaaS product. Could be coaching, portfolio reviews, or job application tooling. The pain point is job searching — not portfolio building.",
        monetization:
            "Weak. Developers don't pay well for job search tools. This space is full of free tools and free advice. Don't target people who don't pay.",
        competitors:
            "LinkedIn, GitHub, every portfolio builder ever made.",
        scores: {
            marketSize: 18,
            painIntensity: 11,
            solutionGap: 8,
            monetization: 7,
        },
        total: 44,
        verdictType: "skip",
        verdictLabel: "Hard pass",
    },
];
