#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
NotebookMind - Mock Test PDF Generator

Generates 8 mock PDF documents for offline evaluation dataset.
Each document contains realistic English content matching eval_dataset.jsonl expectations.
"""

import os

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PROJECT_ROOT = os.path.dirname(SCRIPT_DIR)
OUTPUT_DIR = os.path.join(PROJECT_ROOT, "tests", "pdf")

import sys
try:
    from fpdf import FPDF
except ImportError:
    import subprocess
    subprocess.check_call([sys.executable, "-m", "pip", "install", "fpdf2"])
    from fpdf import FPDF


class PDFDocument(FPDF):
    """Custom PDF generator with consistent styling."""

    def __init__(self, title: str):
        super().__init__(format="letter")  # explicit page format
        self.set_margins(left=20, top=20, right=20)
        self.set_auto_page_break(auto=True, margin=20)
        self.doc_title = title
        self.add_page()
        # Title - use multi_cell to avoid width issues
        self.set_x(20)
        self.set_font("Helvetica", "B", 18)
        self.multi_cell(170, 12, title, align="C")
        self.ln(4)
        # Subtitle
        self.set_x(20)
        self.set_font("Helvetica", "I", 10)
        self.multi_cell(170, 8, "Generated for NotebookMind Evaluation | Fiscal Year 2024", align="C")
        self.ln(6)

    def section(self, title: str):
        self.ln(3)
        self.set_x(20)
        self.set_font("Helvetica", "B", 14)
        self.set_fill_color(240, 240, 245)
        self.cell(170, 10, title, new_x="LMARGIN", new_y="NEXT", fill=True)
        self.ln(2)

    def subsection(self, title: str):
        self.ln(2)
        self.set_x(20)
        self.set_font("Helvetica", "B", 12)
        self.multi_cell(170, 7, title)
        self.ln(1)

    def body_text(self, text: str):
        self.set_x(20)
        self.set_font("Helvetica", "", 10)
        self.multi_cell(170, 5.5, text)

    def bullet_point(self, text: str):
        self.set_x(20)
        self.set_font("Helvetica", "", 10)
        bullet = "  -  "
        self.multi_cell(170, 5.5, bullet + text)

    def add_table(self, headers: list, rows: list, col_widths: list = None):
        self.set_x(20)
        if col_widths is None:
            col_widths = [170 // len(headers)] * len(headers)
        # Ensure total width fits within margins
        total_w = sum(col_widths)
        if total_w > 170:
            scale = 170 / total_w
            col_widths = [w * scale for w in col_widths]
        # Header row
        self.set_font("Helvetica", "B", 9)
        self.set_fill_color(70, 100, 170)
        self.set_text_color(255, 255, 255)
        for i, h in enumerate(headers):
            self.cell(col_widths[i], 8, str(h), border=1, fill=True, align="C")
        self.ln()
        # Data rows
        self.set_text_color(0, 0, 0)
        self.set_font("Helvetica", "", 9)
        fill = False
        for row in rows:
            self.set_x(20)
            if fill:
                self.set_fill_color(248, 248, 252)
            else:
                self.set_fill_color(255, 255, 255)
            for i, cell in enumerate(row):
                align = "C" if i > 0 else "L"
                self.cell(col_widths[i], 7, str(cell), border=1, fill=True, align=align)
            self.ln()
            fill = not fill
        self.ln(2)


def gen_annual_report():
    pdf = PDFDocument("Annual Report 2024 - Nexus Technologies Inc.")

    pdf.section("1. Executive Summary")
    pdf.body_text(
        "Nexus Technologies Inc. delivered exceptional performance in fiscal year 2024, achieving record "
        "revenue of $1.85 billion, representing a 15.3% year-over-year increase from $1.60 billion in 2023. "
        "This growth was driven by strong momentum in our Cloud Infrastructure division (+22% revenue) and "
        "successful market expansion in the Asia-Pacific region. Net income reached $278 million, up 18.5% "
        "from the prior year. The company maintained its position as a leader in enterprise AI platforms with "
        "a 23% market share in the segment."
    )
    pdf.body_text(
        "Our workforce grew from 8,450 employees at year-end 2023 to 9,320 employees at year-end 2024, "
        "reflecting strategic investments in engineering and R&D capabilities. Dr. Sarah Chen, who was "
        "appointed Chief Executive Officer on March 15, 2022, continues to lead the company's transformation "
        "toward AI-first enterprise solutions."
    )

    pdf.section("2. Business Segment Overview")
    pdf.body_text("Nexus Technologies operates across four primary business segments:")
    pdf.bullet_point("Cloud Infrastructure: Provides enterprise cloud computing platforms, container orchestration, and managed Kubernetes services. Revenue: $740 million (40% of total).")
    pdf.bullet_point("AI Platform: Delivers machine learning development tools, model serving infrastructure, and MLOps solutions. Revenue: $463 million (25% of total).")
    pdf.bullet_point("Enterprise Software: Includes data analytics suites, business intelligence tools, and workflow automation products. Revenue: $370 million (20% of total).")
    pdf.bullet_point("Consumer Products: Encompasses personal productivity applications and consumer-facing AI assistants. Revenue: $278 million (15% of total).")

    pdf.subsection("Financial Highlights")
    pdf.add_table(
        ["Metric", "FY2024", "FY2023", "Change"],
        [
            ["Total Revenue", "$1,850M", "$1,605M", "+15.3%"],
            ["Net Income", "$278M", "$235M", "+18.3%"],
            ["Operating Margin", "18.2%", "16.8%", "+1.4pp"],
            ["R&D Investment", "$296M", "$241M", "+22.8%"],
            ["Free Cash Flow", "$342M", "$285M", "+20.0%"],
            ["Employees (FTE)", "9,320", "8,450", "+10.3%"],
        ],
        [55, 45, 45, 45]
    )

    pdf.section("3. Strategic Priorities for FY2025-FY2027")

    pdf.subsection("Three-Year Strategic Goals")
    pdf.body_text(
        "The Board of Directors has approved the following three-year strategic plan, as detailed in our "
        "Strategic_Plan_2024.pdf document:"
    )
    pdf.bullet_point("Goal 1 - Market Leadership: Achieve 30% market share in enterprise AI platform segment by 2027 through product innovation and strategic partnerships.")
    pdf.bullet_point("Goal 2 - Global Expansion: Increase international revenue contribution from 35% to 50% of total revenue by 2027, with focus on Europe and Asia-Pacific markets.")
    pdf.bullet_point("Goal 3 - Operational Excellence: Reduce customer acquisition cost by 25% while maintaining net promoter score above 65 through improved go-to-market efficiency.")

    pdf.section("4. Management Discussion & Analysis")
    pdf.body_text(
        "Revenue growth accelerated in H2 2024 following the launch of NexusAI Pro v3.0 in August 2024, "
        "which introduced real-time collaborative features and advanced RAG capabilities. The Cloud Infrastructure "
        "segment benefited from enterprise digital transformation initiatives, with average contract value increasing "
        "18% year-over-year. Gross margin expanded 120 basis points to 72.4%, driven by optimized infrastructure "
        "costs and a favorable shift toward higher-margin SaaS offerings."
    )
    pdf.body_text(
        "R&D investment increased to 16.0% of revenue (up from 15.0% in 2023), reflecting our commitment to "
        "AI innovation. The engineering team added 420 new positions, primarily in machine learning research, "
        "distributed systems, and security engineering. This investment resulted in 47 new patent filings and "
        "the release of 12 major product updates during the fiscal year."
    )

    pdf.section("5. Risk Factors Summary")
    pdf.body_text(
        "A comprehensive risk assessment has been conducted and documented in Risk_Assessment_2024.pdf. "
        "Key risk categories include: macroeconomic uncertainty affecting enterprise IT spending, cybersecurity "
        "threats requiring ongoing investment in defense measures, talent competition in AI/ML fields, and "
        "regulatory changes affecting data privacy compliance across multiple jurisdictions."
    )

    pdf.output(os.path.join(OUTPUT_DIR, "Annual_Report_2024.pdf"))


def gen_financial_statements():
    pdf = PDFDocument("Consolidated Financial Statements FY2024 - Nexus Technologies Inc.")

    pdf.section("1. Statement of Operations (Income Statement)")
    pdf.body_text("For the Year Ended December 31, 2024 (in thousands USD)")
    pdf.ln(2)
    pdf.add_table(
        ["Line Item", "FY2024", "FY2023"],
        [
            ["Revenue - Products", "1,105,000", "963,000"],
            ["Revenue - Services", "745,000", "642,000"],
            ["Total Revenue", "1,850,000", "1,605,000"],
            ["Cost of Revenue - Products", "(386,750)", "(349,480)"],
            ["Cost of Revenue - Services", "(267,200)", "(236,160)"],
            ["Gross Profit", "1,196,050", "1,019,360"],
            ["Research & Development", "(296,000)", "(241,000)"],
            ["Sales & Marketing", "(370,000)", "(321,000)"],
            ["General & Administrative", "(185,000)", "(161,000)"],
            ["Total Operating Expenses", "(851,000)", "(723,000)"],
            ["Operating Income", "345,050", "296,360"],
            ["Interest Expense", "(18,500)", "(16,050)"],
            ["Other Income (Expense)", "12,300", "8,500"],
            ["Income Before Tax", "338,850", "288,810"],
            ["Income Tax Provision", "(60,993)", "(53,228)"],
            ["Net Income", "277,857", "235,582"],
            ["EPS - Basic", "$30.82", "$27.89"],
            ["EPS - Diluted", "$30.15", "$27.21"],
        ],
        [80, 55, 55]
    )

    pdf.section("2. Quarterly Revenue Breakdown")
    pdf.body_text("Quarterly Performance Analysis (in thousands USD)")
    pdf.ln(2)
    pdf.add_table(
        ["Quarter", "Revenue", "YoY Growth", "Operating Profit"],
        [
            ["Q1 2024", "418,750", "+12.8%", "71,938"],
            ["Q2 2024", "448,125", "+13.5%", "79,663"],
            ["Q3 2024", "481,250", "+15.2%", "88,688"],
            ["Q4 2024", "501,875", "+19.4%", "104,761"],
            ["Full Year", "1,850,000", "+15.3%", "345,050"],
        ],
        [47, 48, 48, 48]
    )
    pdf.body_text(
        "Q4 2024 showed the strongest performance with $501.9M revenue (+19.4% YoY), driven by "
        "year-end enterprise deal closures and the adoption of NexusAI Pro v3.0 launched in Q3. "
        "The sequential growth pattern demonstrates accelerating momentum throughout the year, "
        "with each quarter outperforming the previous one."
    )

    pdf.section("3. Product Line Performance")
    pdf.body_text("Gross Profit Margins by Product Category")
    pdf.ln(2)
    pdf.add_table(
        ["Product Line", "Revenue ($M)", "COGS ($M)", "Gross Profit ($M)", "Margin %"],
        [
            ["Cloud Infrastructure", "740.0", "203.5", "536.5", "72.5%"],
            ["AI Platform", "463.0", "138.9", "324.1", "70.0%"],
            ["Enterprise Software", "370.0", "92.5", "277.5", "75.0%"],
            ["Consumer Products", "277.0", "219.8", "57.2", "20.7%"],
            ["Total", "1,850.0", "654.7", "1,195.3", "64.6%"],
        ],
        [50, 35, 32, 38, 35]
    )
    pdf.body_text(
        "Enterprise Software achieved the highest gross margin at 75.0%, benefiting from high-margin "
        "renewal revenue and minimal delivery costs. Consumer Products carries the lowest margin at 20.7% "
        "due to significant hosting and content delivery costs, though this segment serves as an entry point "
        "for enterprise upselling."
    )

    pdf.section("4. Research & Development Expenditure Breakdown")
    pdf.body_text("R&D Investment Allocation (in thousands USD)")
    pdf.ln(2)
    pdf.add_table(
        ["Category", "FY2024", "FY2023", "Change"],
        [
            ["Basic Research", "88,800", "72,300", "+22.8%"],
            ["Applied Development", "177,600", "144,600", "+22.8%"],
            ["Product Engineering", "19,400", "15,700", "+23.6%"],
            ["Prototyping & Testing", "10,200", "8,400", "+21.4%"],
            ["Total R&D", "296,000", "241,000", "+22.8%"],
        ],
        [60, 45, 45, 40]
    )
    pdf.body_text(
        "Basic research accounts for 30.0% of total R&D spend ($88.8M), focused on foundational AI algorithms, "
        "novel neural network architectures, and long-horizon technology bets. Applied development comprises "
        "60.0% ($177.6M), translating research outputs into shippable product features. The ratio of basic "
        "research to applied development remains constant at 1:2, consistent with our innovation strategy."
    )

    pdf.section("5. Balance Sheet Highlights")
    pdf.body_text("As of December 31, 2024 (in thousands USD)")
    pdf.ln(2)
    pdf.add_table(
        ["Asset/Liability", "FY2024", "FY2023"],
        [
            ["Cash & Equivalents", "685,000", "520,000"],
            ["Accounts Receivable", "312,500", "275,000"],
            ["Property & Equipment, net", "245,000", "210,000"],
            ["Intangible Assets", "380,000", "340,000"],
            ["Total Assets", "2,145,500", "1,850,000"],
            ["Long-term Debt", "420,000", "350,000"],
            ["Shareholders' Equity", "1,395,500", "1,180,000"],
        ],
        [90, 50, 50]
    )
    pdf.body_text(
        "Debt-to-equity ratio improved slightly from 0.297 (2023) to 0.301 (2024), reflecting disciplined "
        "leverage management. The company maintains a revolving credit facility of $200 million, of which "
        "$85 million was drawn at year-end 2024 to fund strategic acquisitions. Management targets a D/E ratio "
        "between 0.25 and 0.35 over the planning horizon."
    )

    pdf.section("6. Cash Flow Statement Summary")
    pdf.body_text("Consolidated Cash Flows (in thousands USD)")
    pdf.ln(2)
    pdf.add_table(
        ["Category", "FY2024", "FY2023"],
        [
            ["Net Income", "277,857", "235,582"],
            ["Depreciation & Amort.", "68,200", "58,500"],
            ["Stock-based Comp.", "(52,000)", "(44,000)"],
            ["Changes in Working Capital", "(42,057)", "(38,082)"],
            ["Cash from Operations", "352,000", "312,000"],
            ["Capital Expenditures", "(98,000)", "(85,000)"],
            ["Acquisitions, net", "(45,000)", "(120,000)"],
            ["Dividends Paid", "(67,000)", "(56,500)"],
            ["Net Change in Cash", "165,000", "108,500"],
        ],
        [80, 55, 55]
    )
    pdf.body_text(
        "Operating cash flow of $352M was sufficient to fully fund capital expenditures ($98M) and dividend "
        "payments ($67M), leaving substantial free cash flow of $187M for debt reduction and opportunistic M&A. "
        "This represents a healthy cash conversion profile, with OCF/revenue at 19.0%, up from 19.4% in 2023 "
        "due to timing of receivable collections."
    )

    pdf.section("7. Notes to Financial Statements")

    pdf.subsection("Note 1: Related Party Transactions")
    pdf.body_text(
        "Related Party Transactions are defined as transactions between the Company and entities or individuals "
        "that have the ability to directly or indirectly control or exercise significant influence over the Company. "
        "During FY2024, related party transactions included: (a) Director fees totaling $1.2 million paid to board "
        "members; (b) Professional services of $3.5 million provided by a firm co-owned by the CFO's spouse; "
        "(c) Lease payments of $2.8 million for office space from a subsidiary of the largest shareholder; "
        "(d) Sub-license revenue of $4.1 million received from an affiliated entity using our technology platform. "
        "All transactions were conducted at arm's length terms as approved by the Audit Committee."
    )

    pdf.output(os.path.join(OUTPUT_DIR, "Financial_Statements_2024.pdf"))


def gen_market_analysis():
    pdf = PDFDocument("Market Analysis Report 2024 - Nexus Technologies Inc.")

    pdf.section("1. Total Addressable Market Overview")
    pdf.body_text(
        "The global enterprise AI platform market reached approximately $62.4 billion in 2024 and is projected "
        "to grow at a CAGR of 28.5% through 2029, reaching $218.7 billion. Nexus Technologies operates primarily in two "
        "segments within this market: Enterprise Cloud Infrastructure ($180B TAM, 12% CAGR) and AI/ML Platform Tools "
        "($42B TAM, 35% CAGR). Our combined addressable opportunity across served markets exceeds $222 billion."
    )

    pdf.section("2. Competitive Landscape & Market Share")
    pdf.body_text("Estimated Market Share Distribution - Enterprise AI Platform Segment (2024)")
    pdf.ln(2)
    pdf.add_table(
        ["Rank", "Company", "Est. Market Share", "Revenue Est."],
        [
            ["1", "DataForge Systems", "28%", "$17.5B"],
            ["2", "CloudScale Corp.", "21%", "$13.1B"],
            ["3", "Nexus Technologies (Us)", "23%", "$14.3B"],
            ["4", "IntellectAI Ltd.", "12%", "$7.5B"],
            ["5", "Synapse Solutions", "8%", "$5.0B"],
            ["Others", "-", "8%", "$5.0B"],
        ],
        [20, 55, 45, 45]
    )
    pdf.body_text(
        "Nexus Technologies holds the #3 position with an estimated 23% market share based on 2024 annualized revenue "
        "in the enterprise AI platform category. Our primary differentiation versus competitors includes: (1) superior "
        "RAG retrieval accuracy benchmarked at 92% vs. industry average of 78%; (2) end-to-end platform covering data "
        "ingestion through model deployment; (3) hybrid cloud flexibility supporting on-premise, private cloud, and public "
        "cloud deployments; (4) domain-specific pre-trained models for healthcare, financial services, and legal industries."
    )

    pdf.section("3. Geographic Revenue Analysis")
    pdf.body_text("Revenue Distribution by Region (FY2024 vs FY2023, in millions USD)")
    pdf.ln(2)
    pdf.add_table(
        ["Region", "FY2024 Revenue", "% of Total", "FY2023 Revenue", "% of Total", "YoY Change"],
        [
            ["North America", "1,202.5", "65.0%", "1,086.7", "67.7%", "+10.7%"],
            ["Europe (EMEA)", "314.5", "17.0%", "256.8", "16.0%", "+22.5%"],
            ["Asia-Pacific", "240.5", "13.0%", "176.5", "11.0%", "+36.3%"],
            ["Latin America", "55.5", "3.0%", "42.3", "2.6%", "+31.2%"],
            ["Other Regions", "37.0", "2.0%", "42.7", "2.7%", "-13.3%"],
            ["Total", "1,850.0", "100.0%", "1,605.0", "100.0%", "+15.3%"],
        ],
        [38, 33, 24, 33, 24, 30]
    )
    pdf.body_text(
        "International revenue (all regions excluding North America) contributed $647.5 million or 35.0% of total "
        "revenue in 2024, up from $518.3 million (32.3%) in 2023. Asia-Pacific was the fastest-growing region at "
        "+36.3% YoY, driven by expansion in Japan, Australia, and India. Europe maintained steady growth at +22.5%, "
        "with Germany and the UK as largest contributing countries. Latin America exceeded expectations with +31.2% "
        "growth, primarily from Brazil and Mexico enterprise adoptions."
    )
    pdf.body_text(
        "Strategic Implication: The geographic mix is shifting favorably toward higher-growth international markets. "
        "Management's target of 50% international revenue by 2027 appears achievable if current APAC and EMEA growth "
        "rates persist. However, this requires continued investment in local sales teams, regional data centers for "
        "compliance, and localized product features including multi-language NLP support."
    )

    pdf.section("4. Competitive Positioning Assessment")
    pdf.subsection("Differentiation Matrix")
    pdf.body_text("Comparison against top 3 competitors on key capability dimensions:")
    pdf.ln(2)
    pdf.add_table(
        ["Capability", "Nexus", "DataForge", "CloudScale"],
        [
            ["RAG Accuracy Score", "92%", "85%", "78%"],
            ["Hybrid Deployment", "Yes", "No", "Yes"],
            ["Multi-modal Support", "Yes", "Partial", "Yes"],
            ["Pre-trained Domains", "5", "3", "4"],
            ["API Latency P95", "1.8s", "2.5s", "2.1s"],
            ["Enterprise Security", "SOC2+ISO", "SOC2", "ISO27001"],
            ["Pricing Flexibility", "High", "Medium", "Low"],
        ],
        [50, 47, 47, 47]
    )

    pdf.section("5. Customer Satisfaction Metrics")
    pdf.subsection("Customer Satisfaction Score (CSAT) Methodology")
    pdf.body_text(
        "Customer Satisfaction Score (CSAT) is measured via quarterly surveys sent to all active enterprise customers "
        "(n=2,847 respondents in Q4 2024). Respondents rate overall satisfaction on a 5-point Likert scale where "
        "1='Very Dissatisfied', 2='Dissatisfied', 3='Neutral', 4='Satisfied', 5='Very Satisfied'. CSAT is calculated as "
        "the percentage of respondents selecting 4 or 5 (Satisfied or Very Satisfied). Survey participation incentive: "
        "respondents receive a 5% discount coupon for one additional user seat."
    )
    pdf.ln(2)
    pdf.add_table(
        ["Metric", "Q1 2024", "Q2 2024", "Q3 2024", "Q4 2024", "Trend"],
        [
            ["CSAT Score", "78.2%", "79.5%", "81.3%", "82.7%", "Improving"],
            ["NPS Score", "54", "57", "61", "63", "Improving"],
            ["Respondents (n)", "2,650", "2,720", "2,790", "2,847", "-"],
            ["Support Ticket Volume", "4,210", "4,050", "3,890", "3,720", "Declining (Good)"],
            ["Avg Resolution Time", "18.2h", "16.8h", "15.1h", "14.3h", "Improving"],
        ],
        [42, 29, 29, 29, 29, 32]
    )
    pdf.body_text(
        "CSAT improved steadily from 78.2% in Q1 to 82.7% in Q4 2024 (+4.5 percentage points). Key drivers identified "
        "through comment analysis include: improved documentation quality (cited 34% of positive comments), reduced API "
        "latency (28%), enhanced admin dashboard (22%), and responsive support team (16%). The NPS score of 63 places us "
        "in the 'Excellent' tier for B2B software (industry average: 42)."
    )

    pdf.output(os.path.join(OUTPUT_DIR, "Market_Analysis_2024.pdf"))


def gen_hr_report():
    pdf = PDFDocument("Human Resources Annual Report FY2024 - Nexus Technologies Inc.")

    pdf.section("1. Workforce Summary")
    pdf.body_text("Employee Headcount as of December 31, 2024")
    pdf.ln(2)
    pdf.add_table(
        ["Department", "Headcount 2024", "Headcount 2023", "Net Change", "% Change"],
        [
            ["Engineering", "4,280", "3,820", "+460", "+12.0%"],
            ["Sales & Marketing", "1,864", "1,710", "+154", "+9.0%"],
            ["Product Management", "648", "580", "+68", "+11.7%"],
            ["Customer Success", "892", "820", "+72", "+8.8%"],
            ["Operations (G&A)", "1,214", "1,140", "+74", "+6.5%"],
            ["Finance & Legal", "242", "220", "+22", "+10.0%"],
            ["Executive Leadership", "18", "16", "+2", "+12.5%"],
            ["Other (HR, Facilities etc.)", "162", "144", "+18", "+12.5%"],
            ["Total FTE", "9,320", "8,450", "+870", "+10.3%"],
        ],
        [50, 35, 35, 32, 30]
    )
    pdf.body_text(
        "Total full-time equivalent headcount increased by 870 employees (+10.3%) year-over-year, reaching 9,320. "
        "Engineering remains the largest department with 4,280 employees (45.9% of total workforce), reflecting the "
        "company's technology-intensive business model. The Engineering department alone added 460 new hires, with "
        "emphasis on ML/AI specialists (120 new hires), backend engineers (150 new hires), and security engineers "
        "(55 new hires)."
    )

    pdf.section("2. Organizational Structure")
    pdf.body_text(
        "The organizational structure follows a functional reporting model under CEO Dr. Sarah Chen. Direct reports "
        "to the CEO include: Chief Technology Officer (Dr. James Wong, leading Engineering with 4,280 staff); Chief "
        "Revenue Officer (Ms. Linda Martinez, leading Sales, Marketing, and Business Development with 1,864 combined "
        "staff); Chief Product Officer (Mr. David Kim, leading Product Management with 648 staff); Chief Customer "
        "Officer (Ms. Angela Foster, leading Customer Success with 892 staff); Chief Operating Officer (Mr. Robert "
        "Chen, leading Operations/G&A with 1,214 staff); Chief Financial Officer (Ms. Patricia Huang, leading Finance "
        "& Legal with 242 staff). Each executive VP oversees 2-4 directors who manage senior managers and team leads. "
        "The typical span of control at the director level is 8-12 direct reports."
    )

    pdf.section("3. Compensation & Benefits Overview")
    pdf.body_text(
        "Total employee compensation expense (including salaries, bonuses, benefits, and stock-based compensation) "
        "was $892 million for FY2024, compared to $764 million in FY2023 (+16.8%). Average compensation per employee "
        "increased from $90,414 to $95,710 (+5.9%), reflecting competitive adjustments in the tech talent market "
        "and merit-based promotion increases. Stock-based compensation represented 18% of total comp expense, "
        "consistent with prior year levels."
    )
    pdf.ln(2)
    pdf.body_text(
        "Revenue per Employee calculation: Total Revenue ($1,850M) / Total Headcount (9,320) = $198,496 per employee. "
        "Compared to FY2023: $1,605M / 8,450 = $189,941 per employee. Productivity per head increased by 4.5% YoY, "
        "indicating improving operational efficiency despite rapid headcount growth. This metric is closely monitored "
        "by management and is included in executive KPI dashboards."
    )

    pdf.section("4. Hiring & Attrition")
    pdf.body_text("Talent Movement Statistics FY2024")
    pdf.ln(2)
    pdf.add_table(
        ["Metric", "Engineering", "All Other Depts", "Company Total"],
        [
            ["New Hires", "512", "428", "940"],
            ["Voluntary Attrition", "168", "215", "383"],
            ["Involuntary Attrition", "28", "42", "70"],
            ["Internal Transfers (net)", "+35", "-35", "0"],
            ["Attrition Rate", "4.4%", "7.2%", "5.5%"],
            ["Avg Tenure (years)", "3.2", "2.8", "3.0"],
        ],
        [55, 45, 45, 45]
    )
    pdf.body_text(
        "Overall voluntary attrition rate of 5.5% compares favorably to the industry average of 8.2% for technology "
        "companies of similar size. Engineering retention (4.4% attrition) is particularly strong due to competitive "
        "compensation, meaningful project assignments, and career development programs. Exit interview data indicates "
        "primary attrition drivers are relocation (32%), career change outside tech industry (24%), startup opportunities "
        "(22%), and work-life balance concerns (18%)."
    )

    pdf.output(os.path.join(OUTPUT_DIR, "HR_Report_2024.pdf"))


def gen_risk_assessment():
    pdf = PDFDocument("Enterprise Risk Assessment FY2024 - Nexus Technologies Inc.")

    pdf.section("1. Risk Register Summary")
    pdf.body_text(
        "The following risk register represents the outcome of the annual enterprise risk assessment conducted "
        "by the Risk Management Committee in October 2024. Each risk is evaluated on a 5-point scale for Likelihood "
        "(1=Rare, 2=Unlikely, 3=Possible, 4=Likely, 5=Almost Certain) and Impact Severity (1=Negligible, 2=Minor, "
        "3=Moderate, 4=Major, 5=Critical). Risk Score = Likelihood x Impact (range 1-25)."
    )
    pdf.ln(2)
    pdf.add_table(
        ["ID", "Risk Factor", "L", "I", "Score", "Status"],
        [
            ["R01", "Macroeconomic Downturn", "3", "4", "12", "Mitigating"],
            ["R02", "Cybersecurity Breach", "3", "5", "15", "Active Monitor"],
            ["R03", "Key Talent Departure", "4", "3", "12", "Mitigating"],
            ["R04", "Regulatory Compliance", "3", "4", "12", "Active Monitor"],
            ["R05", "Competitive Displacement", "3", "3", "9", "Accept"],
            ["R06", "Third-party Dependency", "3", "3", "9", "Mitigating"],
            ["R07", "Data Privacy Violation", "2", "5", "10", "Active Monitor"],
            ["R08", "Technology Obsolescence", "2", "4", "8", "Mitigating"],
            ["R09", "Supply Chain Disruption", "2", "3", "6", "Accept"],
            ["R10", "Currency Exchange Risk", "3", "2", "6", "Hedging"],
        ],
        [15, 62, 12, 12, 18, 35]
    )

    pdf.section("2. Top 5 Detailed Risk Profiles")

    pdf.subsection("R02 - Cybersecurity Breach (Score: 15, HIGH)")
    pdf.body_text(
        "Likelihood: Possible (3) -- Despite robust defenses, the threat landscape evolves continuously. "
        "Impact Severity: Critical (5) -- A breach could result in data exfiltration, regulatory fines exceeding "
        "$50M, reputational damage affecting customer trust, and potential litigation. Mitigation: $28M annual "
        "investment in security operations center, zero-trust architecture implementation, third-party penetration "
        "testing quarterly, employee security awareness training (mandatory annually), and cyber insurance coverage "
        "of $100M. Status: Actively monitored with weekly threat intelligence briefings to the Board."
    )

    pdf.subsection("R01 - Macroeconomic Downturn (Score: 12, ELEVATED)")
    pdf.body_text(
        "Likelihood: Possible (3) -- Economic indicators suggest moderate recession probability (25-35%) over next "
        "18 months. Impact Severity: Major (4) -- Enterprise IT budgets typically contract 10-20% during downturns; "
        "could reduce our revenue by $185-370M. Mitigation: Maintaining diversified customer base (no single customer "
        ">4% of revenue), building recurring revenue base (now 72% of total), preserving cash reserves ($685M), and "
        "developing cost-down playbook enabling 15% OpEx reduction within 90 days if triggered. Materialization Check: "
        "FY2024 results show no materialization yet -- revenue actually accelerated, suggesting current resilience."
    )

    pdf.subsection("R03 - Key Talent Departure (Score: 12, ELEVATED)")
    pdf.body_text(
        "Likelihood: Likely (4) -- Intense competition for AI/ML talent; we've lost 3 senior researchers to competitors "
        "this year. Impact Severity: Moderate (3) -- Loss of critical knowledge holders could delay product roadmaps by "
        "3-6 months. Mitigation: Retention bonus program for top performers (targeting 150 key individuals), equity "
        "refresh grants, internal mobility program enabling cross-team moves, patent naming rights, conference speaking "
        "opportunities, and competitive salary benchmarks reviewed semiannually. Current attrition rate of 5.5% is "
        "below target threshold of 8%."
    )

    pdf.subsection("R04 - Regulatory Non-Compliance (Score: 12, ELEVATED)")
    pdf.body_text(
        "Likelihood: Possible (3) -- Operating in 45 jurisdictions with evolving AI/data regulations (EU AI Act, "
        "state privacy laws). Impact Severity: Major (4) -- Potential fines up to 4% global revenue ($74M), forced "
        "product changes, market access restrictions. Mitigation: Dedicated regulatory affairs team (8 FTEs), "
        "automated compliance monitoring system, external legal counsel retainers in EU, APAC, and North America, "
        "quarterly compliance audits, proactive engagement with regulators on draft rulemaking."
    )

    pdf.subsection("R07 - Data Privacy Violation (Score: 10, MODERATE-HIGH)")
    pdf.body_text(
        "Likelihood: Unlikely (2) -- Strong controls in place but human error or sophisticated attack could bypass. "
        "Impact Severity: Critical (5) -- GDPR fines up to 20M EUR or 4% global revenue; class action exposure; "
        "customer contract terminations. Mitigation: Data minimization policies, automated PII detection in logs, "
        "DSAR response team (SLA: 30 days), Data Protection Officer appointment required under GDPR, annual privacy "
        "impact assessments for new products, encryption-at-rest and in-transit for all customer data."
    )

    pdf.section("3. Risk Materialization Review (FY2024 Actual)")
    pdf.body_text(
        "Retrospective assessment of which risks materialized during FY2024 and effectiveness of mitigations:"
    )
    pdf.ln(2)
    pdf.add_table(
        ["Risk ID", "Materialized?", "Actual Impact", "Mitigation Effectiveness"],
        [
            ["R01 (Macro)", "No", "N/A", "N/A -- Economy remained resilient"],
            ["R02 (Cyber)", "Partially", "1 minor incident, $0 damage", "Effective -- SOC2 audit passed clean"],
            ["R03 (Talent)", "Yes", "3 senior departures, 2-month delay", "Moderately effective -- backfilled in 60 days"],
            ["R04 (Regulatory)", "No", "N/A", "Proactive -- EU AI Act prep underway"],
            ["R05 (Competitive)", "Partially", "Lost 2 deals to DataForge", "Acceptable -- win rate still 73%"],
            ["R06 (3rd Party)", "No", "N/A", "Vendor diversification worked well"],
            ["R07 (Privacy)", "No", "N/A", "Zero DSAR escalations in FY2024"],
            ["R08 (Tech Obs.)", "No", "N/A", "R&D pipeline remains strong"],
            ["R09 (Supply Chain)", "No", "N/A", "Multi-cloud strategy proved resilient"],
            ["R10 (Currency)", "Minor", "$4.2M FX headwind", "Hedging covered 80% of exposure"],
        ],
        [28, 30, 52, 80]
    )

    pdf.output(os.path.join(OUTPUT_DIR, "Risk_Assessment_2024.pdf"))


def gen_strategic_plan():
    pdf = PDFDocument("Strategic Plan 2025-2027 - Nexus Technologies Inc.")

    pdf.section("1. Vision & Mission Statement")
    pdf.body_text(
        "Vision: To be the most trusted and capable AI-powered knowledge platform for enterprises worldwide, "
        "empowering every organization to unlock the full value of their data through intelligent, accurate, "
        "and actionable insights.\n\n"
        "Mission: We build enterprise-grade AI platforms that combine state-of-the-art retrieval-augmented generation "
        "with deep domain expertise, enabling our customers to make better decisions faster while maintaining complete "
        "control over their data and intellectual property."
    )

    pdf.section("2. Three-Year Strategic Goals (Approved by Board, November 2024)")

    pdf.subsection("Goal 1: Market Leadership (Source: Strategic_Plan_2024.pdf, Page 3, Section 2.1)")
    pdf.body_text(
        "Objective: Achieve 30% market share in the enterprise AI platform segment by December 2027, up from the "
        "current estimated 23%. Key initiatives include: (a) Launch vertical-specific solutions for healthcare, "
        "financial services, and legal sectors targeting $12B combined TAM; (b) Establish strategic partnerships with "
        "top 5 global system integrators (Accenture, Deloitte, IBM Consulting, Capgemini, Wipro); (c) Invest $150M "
        "in brand building and thought leadership over 3 years; (d) Target Fortune 500 penetration increase from 34% "
        "to 55%. Success Metric: External analyst-reported market share >= 30% by Dec 2027."
    )

    pdf.subsection("Goal 2: Global Expansion (Source: Strategic_Plan_2024.pdf, Page 4, Section 2.2)")
    pdf.body_text(
        "Objective: Increase international revenue contribution from 35% to 50% of total revenue by December 2027. "
        "Key initiatives: (a) Establish dedicated regional headquarters in London (EMEA HQ), Singapore (APAC HQ), "
        "and Sao Paulo (LATAM hub); (b) Build regional data centers in Frankfurt, Tokyo, and Sydney for data residency "
        "compliance; (c) Localize product UI into 12 languages with native NLP models for Japanese, German, French, "
        "Spanish, Portuguese, Korean, and Mandarin Chinese; (d) Grow APAC revenue from $240M (13% of total) to $550M "
        "(~22% of projected total) by 2027. Success Metric: International revenue >= 50% of consolidated revenue."
    )

    pdf.subsection("Goal 3: Operational Excellence (Source: Strategic_Plan_2024.pdf, Page 5, Section 2.3)")
    pdf.body_text(
        "Objective: Reduce Customer Acquisition Cost (CAC) by 25% while maintaining Net Promoter Score (NPS) above 65. "
        "Key initiatives: (a) Implement product-led growth motion with free tier and self-service upgrade path, "
        "targeting 40% of SMB acquisitions via PLG by 2027; (b) Expand partner ecosystem to generate 35% of new logos "
        "via channel partners (currently 18%); (c) Deploy AI-powered sales assistant for lead scoring and outreach "
        "personalization, targeting 2x improvement in sales rep productivity; (d) Enhance customer success programs "
        "to drive net dollar retention above 115%. Success Metrics: CAC <= $12,500 (from ~$16,700 today), NPS >= 65."
    )

    pdf.section("3. Strategic Initiatives Roadmap")

    pdf.subsection("Year 1 (2025): Foundation Building")
    pdf.bullet_point("Complete EU AI Act compliance certification for all product lines (Q2 2025)")
    pdf.bullet_point("Launch Healthcare AI Suite with HIPAA-compliant architecture (Q3 2025)")
    pdf.bullet_point("Open Singapore regional office and begin APAC localization (Q1 2025)")
    pdf.bullet_point("Ship NexusAI Pro v4.0 with multi-agent reasoning capabilities (Q4 2025)")
    pdf.bullet_point("Achieve FedRAMP High authorization for government sector (Q4 2025)")

    pdf.subsection("Year 2 (2026): Scaling Phase")
    pdf.bullet_point("Launch Financial Services AI solution with regulatory reporting integration (Q1 2026)")
    pdf.bullet_point("Expand partner program to 200 certified system integrators globally (Q2 2026)")
    pdf.bullet_point("Open London EMI headquarters and Frankfurt data center (H1 2026)")
    pdf.bullet_point("Introduce consumption-based pricing alongside existing subscription model (Q3 2026)")

    pdf.subsection("Year 3 (2027): Market Dominance")
    pdf.bullet_point("Target 30% enterprise AI platform market share (year-end validation)")
    pdf.bullet_point("Achieve 50% international revenue contribution milestone")
    pdf.bullet_point("Launch Legal AI solution with case law analysis and contract review (Q2 2027)")
    pdf.bullet_point("Evaluate strategic M&A opportunities for capability gaps or market entry acceleration")

    pdf.section("4. Resource Requirements & Investment Plan")
    pdf.body_text("Three-Year Capital Allocation (in millions USD)")
    pdf.ln(2)
    pdf.add_table(
        ["Investment Area", "2025", "2026", "2027", "3-Yr Total"],
        [
            ["R&D / Product Innovation", "340", "395", "450", "1,185"],
            ["Sales & Marketing Expansion", "290", "330", "365", "985"],
            ["International Infrastructure", "120", "150", "110", "380"],
            ["M&A Reserve Fund", "100", "150", "200", "450"],
            ["Operational Capacity", "80", "95", "110", "285"],
            ["Total Incremental Inv.", "930", "1,120", "1,235", "3,285"],
        ],
        [55, 35, 35, 35, 35]
    )

    pdf.output(os.path.join(OUTPUT_DIR, "Strategic_Plan_2024.pdf"))


def gen_process_guidelines():
    pdf = PDFDocument("Internal Process Guidelines & Governance Policies v4.2")

    pdf.section("1. Product Development Lifecycle (PDLC)")

    pdf.body_text(
        "All product development activities must follow the standardized PDLC process defined below. Deviations require "
        "written approval from the Chief Product Officer. The process consists of seven phases, each with defined deliverables "
        "and mandatory approval gates."
    )

    pdf.ln(2)
    pdf.add_table(
        ["Phase", "Activities", "Duration", "Approval Gate"],
        [
            ["1. Ideation", "Problem discovery, market research, user interviews", "2-4 wks", "PM Director sign-off"],
            ["2. Feasibility", "Technical assessment, resource estimation, ROI model", "2-3 wks", "CTO + CFO joint approval"],
            ["3. Design", "PRD writing, UX wireframes, technical spec", "3-6 wks", "Design Review Board (DRB)"],
            ["4. Development", "Sprint-based implementation, code reviews", "8-16 wks", "Engineering Manager QA sign-off"],
            ["5. Testing", "Unit/integration testing, security scan, UAT", "3-6 wks", "QA Lead + Security Team approval"],
            ["6. Staging", "Performance testing, staging env validation, docs", "2-3 wks", "Release Manager approval"],
            ["7. Launch", "GA release, customer comms, post-launch monitoring", "1-2 wks", "Go/No-Go Committee (VP+)"],
        ],
        [25, 70, 25, 50]
    )

    pdf.body_text(
        "Total typical cycle time: 21-40 weeks depending on scope complexity (Small: <21 wks, Medium: 21-30 wks, "
        "Large: 30-40 wks, X-Large: 40+ wks requiring exec steering committee oversight). Each gate requires formal "
        "documentation submitted to the designated approver with a maximum 5-business-day SLA for approval decisions. "
        "Gate rejections trigger a root cause analysis and revised plan submission within 10 business days."
    )

    pdf.section("2. Vendor Procurement & Approval Workflow")

    pdf.body_text(
        "All vendor engagements must follow the procurement policy below to ensure cost control, quality assurance, "
        "and compliance with legal requirements. Procurement is categorized by annual contract value (ACV)."
    )

    pdf.ln(2)
    pdf.add_table(
        ["ACV Tier", "Approval Authority", "Required Steps"],
        [
            ["<$10K", "Department Manager", "Quote comparison, single-vendor OK"],
            ["$10K-$50K", "Director + Finance review", "Min 2 quotes, standard contract template"],
            ["$50K-$200K", "VP + Procurement lead", "Min 3 quotes, legal review, security assessment"],
            ["$200K-$500K", "SVP + CFO approval", "Formal RFP, vendor background check, reference calls"],
            [">$500K", "CEO + Board notification", "Full RFP process, exec presentation, Board memo"],
        ],
        [35, 55, 100]
    )

    pdf.body_text(
        "Expedited Process: For time-critical procurements (<5 day deadline), one-tier elevation is permitted with "
        "documented business justification, subject to post-procurement audit within 30 days. Preferred vendors "
        "(pre-negotiated framework agreements) may bypass quote comparison requirements but not approval thresholds. "
        "All contracts exceeding $50K ACV automatically route through Legal for terms review regardless of tier."
    )

    pdf.section("3. Code Review & Release Policy")

    pdf.subsection("Code Review Requirements")
    pdf.bullet_point("All production code changes require at least one peer approval from a qualified reviewer.")
    pdf.bullet_point("Security-sensitive code (auth, crypto, PII handling) requires dedicated Security Team review.")
    pdf.bullet_point("Changes affecting >3 modules or >500 lines require Architect sign-off before merge.")
    pdf.bullet_point("Hotfixes to production require on-call engineer + on-call manager approval, retroactive full review within 48 hours.")
    pdf.bullet_point("External open-source dependencies must be vetted by Security Team for license compliance and vulnerability scan.")

    pdf.subsection("Release Cadence Policy")
    pdf.body_text(
        "SaaS Platform releases follow a bi-weekly cadence (every other Tuesday). Feature freeze occurs 5 business days "
        "prior to release date. Critical security patches follow an emergency release path with same-day deployment "
        "capability (requires CTO approval + incident ticket). All releases must pass: unit test coverage >= 80%, zero "
        "critical/high severity static analysis findings, performance regression test (<5% degradation), and manual QA "
        "sign-off from the assigned QA engineer."
    )

    pdf.section("4. Data Handling & Classification Policy")
    pdf.body_text("All company data is classified into four tiers with corresponding handling requirements:")
    pdf.ln(2)
    pdf.add_table(
        ["Classification", "Examples", "Access Control", "Retention"],
        [
            ["Public", "Marketing materials, pricing page", "Unrestricted", "Indefinite"],
            ["Internal", "Policies, org charts, non-financial reports", "All employees", "7 years"],
            ["Confidential", "Financial data, customer lists, strategies", "Need-to-know basis", "7 years"],
            ["Restricted", "PII, credentials, security configs, M&A docs", "Named individuals only", "Per regulation"],
        ],
        [32, 60, 50, 38]
    )

    pdf.output(os.path.join(OUTPUT_DIR, "Process_Guidelines.pdf"))


def gen_org_chart():
    pdf = PDFDocument("Organizational Structure Chart FY2024 - Nexus Technologies Inc.")

    pdf.section("Executive Leadership Team")
    pdf.body_text(
        "The following organizational chart depicts the reporting structure of Nexus Technologies Inc. as of "
        "December 31, 2024. The organization follows a functional hierarchy with clear spans of control and "
        "defined decision-making authority at each level."
    )
    pdf.ln(4)

    # Visual text-based org chart
    pdf.set_font("Courier", "", 7)
    pdf.set_x(10)

    chart_lines = [
        "",
        "                          +-----------------------------+",
        "                          |      CEO                    |",
        "                          |   Dr. Sarah Chen             |",
        "                          |   Direct Reports: 6          |",
        "                          +--------------+--------------+",
        "                                         |",
        "         +---------------+------+------+------+---------------+",
        "         |               |             |      |               |",
        "  +------+-------+ +-----+------+ +----+---+ +-+--------+ +--+--------+",
        "  | CTO          | | CRO         | | CPO     | | CCO       | | COO       |",
        "  | Dr.James Wong| | L.Martinez  | |D.Kim    | |A.Foster   | |R.Chen     |",
        "  | Reports: 5    | | Reports: 3  | |Reports:2| |Reports:3  | |Reports:4  |",
        "  | Staff: 4,280  | | Staff:1,864 | |Staff:648| |Staff:892  | |Staff:1,214|",
        "  +------+-------+ +------+-------+ +----+----+ +----------+ +-----+-----+",
        "         |                |              |           |              |",
        "  +------+------+   +-----+------+  +----+----+ +--+--------+  +----+-----+",
        "  |      |      |   |     |      |  |         | |           |  |    |     |",
        "[VP    [VP   [VP   [VP   [VP   [Dir] [Dir] [Dir][Dir]      [Dir][Dir][Dir][Dir]",
        " Eng]  Infra] Sec]  Sales Mktg]  PM] Prod] Des] CS ]  Ops] HR ] Fin] Legal]Fac]",
        "",
        "  Legend:",
        "  ------ Direct solid line = Reporting relationship",
        "  ...... Dotted line   = Cross-functional / Matrix relationship",
        "",
        "  Level 0: CEO (1 person)",
        "  Level 1: C-Suite Executives (6 persons)",
        "  Level 2: VPs & Directors (22 persons)",
        "  Level 3: Senior Managers (~85 persons)",
        "  Level 4: Managers & Team Leads (~320 persons)",
        "  Level 5: Individual Contributors (~8,886 persons)",
        "",
        "  Average Span of Control by Level:",
        "    Level 0->1: 6 direct reports (CEO to C-suite)",
        "    Level 1->2: avg 3.7 direct reports per executive",
        "    Level 2->3: avg 3.9 direct reports per VP/Director",
        "    Level 3->4: avg 3.8 direct reports per Senior Manager",
        "    Level 4->5: avg 27.8 direct reports per Manager/Lead",
    ]

    for line in chart_lines:
        pdf.multi_cell(190, 4, line)

    pdf.ln(3)
    pdf.set_font("Helvetica", "", 10)

    pdf.section("Detailed Role Descriptions")

    pdf.subsection("Chief Technology Officer - Dr. James Wong")
    pdf.body_text(
        "Dr. James Wong reports directly to the CEO and oversees the entire Technology organization comprising 4,280 "
        "employees. His five direct reports include: (1) VP of Engineering (infrastructure and platform teams, 1,850 staff); "
        "(2) VP of Information Security (security engineering and operations, 320 staff); (3) VP of AI Research (ML/AI "
        "research labs, 480 staff); (4) VP of Data Engineering (data pipelines and analytics platform, 620 staff); "
        "(5) VP of Developer Experience (developer tools, CI/CD platform, internal systems, 310 staff). "
        "Dr. Wong holds a PhD in Distributed Systems from Stanford University and previously served as VP of Engineering "
        "at CloudScale Corp. for 8 years before joining Nexus in 2019."
    )

    pdf.subsection("Chief Revenue Officer - Ms. Linda Martinez")
    pdf.body_text(
        "Ms. Linda Martinez reports directly to the CEO and leads all revenue-generating functions with 1,864 employees. "
        "Her three direct reports: (1) VP of Enterprise Sales (field sales, account executives, solutions engineers, 920 staff); "
        "(2) VP of Marketing (demand generation, brand, product marketing, events, 580 staff); (3) VP of Business Development "
        "& Partnerships (strategic alliances, channel sales, M&A sourcing, 364 staff). Ms. Martinez joined Nexus in 2021 from "
        "her former role as SVP of Worldwide Sales at IntellectAI Ltd., bringing 18 years of enterprise sales leadership experience."
    )

    pdf.subsection("Chief Product Officer - Mr. David Kim")
    pdf.body_text(
        "Mr. David Kim reports to the CEO and leads Product Management with 648 employees across two divisions: (1) Director of "
        "Core Products (NexusAI platform, document processing, search, 380 staff); (2) Director of Industry Solutions (vertical "
        "products for specific industries, 268 staff). He works closely with Engineering (CTO) and Go-to-Market (CRO) to ensure "
        "product-market fit and roadmap alignment. Mr. Kim previously led product management at DataForge Systems for 7 years."
    )

    pdf.subsection("Chief Customer Officer - Ms. Angela Foster")
    pdf.body_text(
        "Ms. Angela Foster reports to the CEO and owns the post-sale customer experience with 892 employees: (1) Director of "
        "Customer Success (enterprise CSM team, adoption, renewals, 520 staff); (2) Director of Technical Support (tiered "
        "support, knowledge base, 260 staff); (3) Director of Professional Services (implementation consulting, 112 staff). "
        "Her organization is measured on Net Dollar Retention (target: >115%), NPS (target: >65), and Time-to-Value (target: "
        "<30 days for enterprise deployments)."
    )

    pdf.subsection("Chief Operating Officer - Mr. Robert Chen")
    pdf.body_text(
        "Mr. Robert Chen (no relation to CEO) reports to the CEO and runs corporate operations with 1,214 employees spanning: "
        "(1) Director of IT & Workplace Technology (internal systems, facilities tech, 180 staff); (2) Director of People & Talent "
        "(HR, recruiting, L&D, 340 staff); (3) Director of Finance Operations (FP&A, accounting, treasury, 280 staff); "
        "(4) Director of Corporate Affairs (legal, compliance, communications, 220 staff); (5) Director of Program Management Office "
        "(cross-functional initiative coordination, 94 staff). Mr. Chen has been with Nexus since 2015 and previously held "
        "operational roles at two Fortune 500 companies."
    )

    pdf.subsection("Chief Financial Officer - Ms. Patricia Huang")
    pdf.body_text(
        "Ms. Patricia Huang reports to the CEO and leads Finance & Legal functions with 242 employees. Her organization includes: "
        "(1) Controller team (financial reporting, tax, audit coordination, 65 staff); (2) Treasury team (cash management, "
        "investor relations, banking relationships, 38 staff); (3) Legal team (corporate law, IP, contracts, regulatory, 85 staff); "
        "(4) Internal Audit & Risk (SOX compliance, operational audits, 54 staff). Ms. Huang is a CPA with 20 years of experience, "
        "including 8 years in public accounting at a Big Four firm and 12 years as a public company CFO."
    )

    pdf.output(os.path.join(OUTPUT_DIR, "Organization_Chart.pdf"))


def main():
    os.makedirs(OUTPUT_DIR, exist_ok=True)
    print(f"Output directory: {os.path.abspath(OUTPUT_DIR)}\n")

    generators = [
        ("Annual_Report_2024.pdf", gen_annual_report),
        ("Financial_Statements_2024.pdf", gen_financial_statements),
        ("Market_Analysis_2024.pdf", gen_market_analysis),
        ("HR_Report_2024.pdf", gen_hr_report),
        ("Risk_Assessment_2024.pdf", gen_risk_assessment),
        ("Strategic_Plan_2024.pdf", gen_strategic_plan),
        ("Process_Guidelines.pdf", gen_process_guidelines),
        ("Organization_Chart.pdf", gen_org_chart),
    ]

    for name, gen_fn in generators:
        try:
            gen_fn()
            size_kb = os.path.getsize(os.path.join(OUTPUT_DIR, name)) / 1024
            print(f"  [OK] {name} ({size_kb:.1f} KB)")
        except Exception as e:
            print(f"  [FAIL] {name}: {e}")

    print(f"\nDone! {len(generators)} PDF documents generated in: {os.path.abspath(OUTPUT_DIR)}")


if __name__ == "__main__":
    main()
