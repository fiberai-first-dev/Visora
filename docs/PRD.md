We are building an AI-powered SEO + GEO intelligence platform specifically for Indian D2C brands.

## Product Vision

Build a platform where a D2C brand enters only its website URL. The platform automatically:

1. Crawls and understands the website.
2. Performs technical SEO analysis.
3. Extracts products, categories, topics, entities and content.
4. Understands the brand's search intent and market.
5. Automatically discovers relevant competitors.
6. Analyzes competitor SEO/content/search visibility.
7. Measures the brand's visibility in AI search/answer engines.
8. Identifies SEO and GEO gaps.
9. Generates actionable recommendations.
10. Continuously monitors the brand and its competitive/search landscape for changes.

The customer should NOT have to manually provide competitors.

---

# 1. Core Product Flow

```text
User enters website
        ↓
Website crawler
        ↓
Page extraction
        ↓
Technical SEO analysis
        ↓
Content / product / topic understanding
        ↓
Brand profile
        ↓
Search landscape discovery
        ↓
Automatic competitor discovery
        ↓
Competitor analysis
        ↓
GEO / AI search analysis
        ↓
SEO + GEO opportunities
        ↓
Actionable recommendations
        ↓
Continuous monitoring
```

---

# 2. Indian D2C Brand Registry

Maintain a broad registry of Indian D2C brands.

The registry should NOT require continuously crawling every brand in the country.

Initially store lightweight metadata:

```text
brand
domain
category
sub_category
product_types
price_range
target_market
country
```

The registry acts as a candidate universe for competitor discovery.

Possible categories:

* Beauty
* Skincare
* Haircare
* Fashion
* Food & Beverage
* Fitness
* Wellness
* Home
* Electronics
* Personal Care
* Pet Products
* Lifestyle
* Other D2C categories

The registry can be populated from publicly available sources and automated discovery.

Do not assume the registry is complete.

---

# 3. Website Crawling

When a user enters:

```text
https://example.com
```

create a project and crawl the website.

First inspect:

```text
/robots.txt
/sitemap.xml
```

Use the sitemap when available.

Otherwise discover internal URLs by crawling links.

Respect:

* robots.txt
* crawl restrictions
* reasonable rate limits
* website/server resources
* applicable terms and policies

Do not aggressively crawl websites.

---

# 4. Page Extraction

For every discovered page extract structured information:

```text
url
status_code
title
meta_description
canonical
robots_directive
h1
headings
clean_text
word_count
internal_links
external_links
images
image_alt_text
structured_data
language
content_type
```

Classify pages into types:

```text
homepage
product
collection/category
blog/article
landing page
FAQ
about
contact
other
```

For D2C websites, product and collection pages are especially important.

---

# 5. Technical SEO Engine

Build a deterministic rule-based SEO analyzer.

Analyze:

### Crawlability

* robots.txt
* sitemap
* HTTP status
* redirects
* crawl errors

### Indexability

* noindex
* canonical
* duplicate URLs
* canonical conflicts

### Metadata

* missing titles
* duplicate titles
* title length
* missing meta descriptions
* duplicate meta descriptions
* H1 issues

### Content

* thin content
* duplicate content
* heading structure
* content coverage

### Links

* broken internal links
* orphan pages
* internal linking
* excessive redirects

### Images

* missing alt text
* image size
* image metadata where available

### Structured data

Detect relevant schema such as:

* Product
* Organization
* Breadcrumb
* Article
* FAQ
* Review
* Offer

Do NOT claim that fixing any individual issue guarantees a ranking improvement.

The system should describe these as SEO opportunities/issues based on documented search-engine guidance and observed website characteristics.

---

# 6. Brand Understanding Engine

Use the extracted website data to build a structured brand profile.

Identify:

```text
brand name
category
subcategories
products
product types
price ranges
target audience
locations
topics
entities
claims
USPs
```

Example:

```text
Brand: XYZ

Category: Skincare

Products:
- Vitamin C Serum
- Niacinamide Serum
- Sunscreen

Topics:
- pigmentation
- acne
- oily skin
- skincare routines
```

Use deterministic extraction where possible and LLMs only where semantic understanding is required.

---

# 7. Search Intent / Topic Engine

From the brand profile generate relevant search intents and topic clusters.

Classify intents:

```text
informational
commercial
transactional
navigational
```

Example:

```text
Product:
Vitamin C Serum

Potential intents:

vitamin c serum benefits
→ informational

best vitamin c serum india
→ commercial

buy vitamin c serum
→ transactional
```

Do not treat simple keyword frequency as the entire SEO strategy.

Focus on:

* topics
* entities
* search intent
* content coverage
* semantic relationships

---

# 8. Automatic Competitor Discovery

The customer must NOT manually provide competitors.

Discover competitors using multiple signals.

### Signal 1 — Brand Registry

Find brands with:

* similar categories
* similar products
* similar price range
* similar market
* similar audience

### Signal 2 — Search Landscape

Generate important queries for the brand.

Analyze search results and identify domains that repeatedly appear.

### Signal 3 — Product Similarity

Compare the customer's products/categories with candidate brands.

### Signal 4 — Topic/Search Overlap

Measure how frequently a candidate appears for relevant topics and queries.

### Signal 5 — Domain Classification

Classify domains as:

```text
D2C brand
marketplace
publisher
review website
affiliate
community
retailer
manufacturer
other
```

Do not automatically classify every high-ranking domain as a competitor.

Separate:

```text
Direct competitors
Search competitors
Content competitors
```

Example:

```text
Direct competitor:
similar products + similar audience + similar market

Search competitor:
competes for the same search queries

Content competitor:
competes for informational topics
```

The system should present competitor discovery as a data-driven candidate analysis, not an absolute truth.

---

# 9. Competitor Deep Analysis

After discovering candidate competitors, deeply analyze only the most relevant candidates.

Do not continuously crawl every possible competitor.

Compare:

```text
site structure
product coverage
category coverage
content topics
technical SEO
structured data
internal linking
content depth
search visibility
GEO visibility
```

Generate content/topic gaps.

Example:

```text
Competitor:
- acne
- oily skin
- sensitive skin
- anti-aging

Your brand:
- vitamin C
- niacinamide
- sunscreen

Potential content gaps:
- acne
- oily skin
- sensitive skin
- anti-aging
```

These are opportunities, not guaranteed ranking factors.

---

# 10. GEO / AI Search Engine

Build a GEO intelligence system that measures how often a brand appears in AI-generated answers.

Generate relevant prompts based on:

```text
brand
products
category
topics
search intent
location
```

Examples:

```text
What are the best Indian skincare brands?

What is the best vitamin C serum in India?

Which sunscreen is good for oily skin?

What are affordable Indian skincare brands?
```

Run these prompts against supported AI search/answer providers through appropriate APIs or permitted interfaces.

Store:

```text
prompt
provider/model
timestamp
response
brand_mentioned
brand_position
competitors_mentioned
citations
source_domains
context/sentiment
```

Do not assume different AI systems produce identical results.

Track each provider separately.

---

# 11. GEO Metrics

Track metrics such as:

### Brand Mention Rate

```text
responses mentioning brand
/
total responses
```

### Citation Rate

```text
responses citing brand-owned sources
/
total responses
```

### Competitor Share of Voice

Measure relative brand mentions across the tracked prompt set.

### AI Position

Where the brand appears in the answer when meaningful.

### Citation Sources

Identify domains frequently cited in AI answers.

Do not reduce GEO to one arbitrary score.

Always allow users to inspect the underlying prompts and responses.

---

# 12. Citation Intelligence

Analyze which sources are commonly cited by AI systems for the customer's category.

Example:

```text
AI answer sources:

Reddit
YouTube
Brand websites
Review sites
News websites
Blogs
Ecommerce sites
```

Compare the customer's presence across these source types.

Generate recommendations such as:

```text
Your brand has strong first-party content,
but limited third-party references in sources
frequently appearing in AI answers.
```

Do not claim that a particular external source guarantees AI visibility.

---

# 13. Continuous Monitoring

This is a core product feature.

Do NOT crawl every page continuously at the same frequency.

Use different schedules:

```text
Homepage             daily
Product pages        daily / every few days
Collections          daily / weekly
Blog articles        weekly
Static pages         weekly
```

Use change detection:

```text
previous snapshot
       ↓
content/hash comparison
       ↓
changed?
  /       \
no         yes
 |          |
skip       analyze
```

Monitor:

```text
new products
removed products
price changes
content changes
title changes
meta changes
schema changes
new blog posts
deleted pages
canonical changes
indexability changes
SEO issues
search visibility
GEO visibility
competitor changes
```

---

# 14. Search Landscape Monitoring

Continuously track important queries.

Detect:

```text
new competitors
competitor visibility increases
competitor visibility decreases
new ranking domains
new content
new search opportunities
changes in AI answers
changes in citations
```

Example alert:

```text
New competitor detected

Brand X has appeared repeatedly
for 18 queries relevant to your products
over the last 30 days.

Product overlap: High
Search overlap: High
Category overlap: High
```

---

# 15. Recommendation Engine

The final output should not just be a list of SEO errors.

It should answer:

> What should the brand do next?

Examples:

```text
HIGH PRIORITY

Product page has missing Product structured data.

ACTION:
Add valid Product schema containing
product name, image, price and availability.
```

Another:

```text
CONTENT OPPORTUNITY

Competitors consistently cover:
"best sunscreen for oily skin"

Your website has no strong page
addressing this intent.

ACTION:
Create a dedicated resource targeting
this commercial/informational intent.
```

Another:

```text
GEO OPPORTUNITY

Your brand is rarely mentioned in AI answers
for relevant category prompts.

Several frequently cited third-party source
types have little or no coverage of your brand.

ACTION:
Investigate authoritative third-party
coverage and strengthen first-party
topic/product information.
```

Avoid claims like:

```text
"Do this and your ranking will increase 30%."
```

---

# 16. Dashboard

Initial dashboard:

```text
Overview
SEO
GEO
Search Landscape
Competitors
Content Opportunities
Recommendations
Monitoring
Settings
```

Overview should show:

```text
SEO health
GEO visibility
Search visibility
Competitor landscape
Top opportunities
Recent changes
```

Example:

```text
SEARCH VISIBILITY

SEO Health             78
GEO Mention Rate       42%
Citation Rate          31%

Top Opportunities
1. Product schema issue
2. Missing topic coverage
3. Weak internal linking
4. Emerging competitor
5. GEO citation gap
```

Do not present arbitrary scores as objective Google rankings or guaranteed outcomes. Explain what each metric represents.

---

# 17. Architecture

Recommended initial architecture:

```text
Frontend
Next.js
TypeScript
Tailwind
shadcn/ui
Zustand
TanStack Query

Backend
Go (Rest APIs and Background Workers using Colly/GORM)

Storage
PostgreSQL
Object storage for large crawl artifacts

Infrastructure
Docker initially
Kubernetes only when scale requires it
```

Architecture:

```text
                Next.js
                   │
                   ▼
                API (Go)
                   │
       ┌───────────┼───────────┐
       ▼           ▼           ▼
   PostgreSQL    Redis      RabbitMQ
                               │
              ┌────────────────┼────────────────┐
              ▼                ▼                ▼
         Crawl Worker      SEO Worker       GEO Worker
             (Go)             (Go)             (Go)
              │                │                │
              ▼                ▼                ▼
          Websites          Analysis        AI APIs
```

---

# 18. Database

Start with entities such as:

```text
organizations
users
projects
websites

brands
brand_categories
brand_products

pages
crawl_runs
page_snapshots

seo_issues
topics
keywords
search_queries

competitor_candidates
competitor_relationships

geo_prompts
geo_runs
geo_mentions
geo_citations

recommendations
content_opportunities

change_events
integrations
```

Do not over-engineer the database before validating the product.

---

# 19. MVP Scope

Build the first version around this workflow:

```text
Website URL
    ↓
Crawl
    ↓
Technical SEO audit
    ↓
Extract products/topics/content
    ↓
Generate search intents
    ↓
Discover competitors automatically
    ↓
Basic competitor/content-gap analysis
    ↓
GEO prompt generation
    ↓
AI visibility measurement
    ↓
Actionable recommendations
```

Do NOT initially build:

* full Ahrefs-style backlink database
* massive keyword database
* dozens of integrations
* social media management
* CRM
* ad management
* generic AI content writer
* automated publishing everywhere
* global market support
* massive Kubernetes infrastructure

Start with Indian D2C brands.

---

# 20. Long-Term Product

The final product should become a continuous search intelligence system:

```text
                D2C BRAND
                    │
                    ▼
                WEBSITE
                    │
                    ▼
                 CRAWL
                    │
       ┌────────────┼────────────┐
       ▼            ▼            ▼
      SEO        CONTENT       PRODUCTS
       │            │            │
       └────────────┼────────────┘
                    ▼
             SEARCH LANDSCAPE
                    │
          ┌─────────┴─────────┐
          ▼                   ▼
    COMPETITORS             GEO
          │                   │
          └─────────┬─────────┘
                    ▼
                 ANALYZE
                    │
                    ▼
              FIND GAPS
                    │
                    ▼
             RECOMMENDATIONS
                    │
                    ▼
                 CHANGES
                    │
                    ▼
              RE-MEASURE
                    │
                    └──────────► CONTINUOUS LOOP
```

The central product promise is:

> **Enter your D2C website once. We continuously understand your website, search landscape, competitors, and AI visibility — and tell you what changed and what you should do next.**
