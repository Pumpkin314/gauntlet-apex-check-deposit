-- Seed data for Apex Mobile Check Deposit
-- 2 correspondents, 12 accounts per PRD 4.2

-- Correspondents
INSERT INTO correspondents (id, code, name, rules_config) VALUES
(
    'c0000000-0000-0000-0000-000000000001',
    'ALPHA',
    'Alpha Brokerage',
    '{
        "deposit_limit": 5000,
        "ineligible_account_types": [],
        "contribution_cap": 7000,
        "fees": {
            "returned_check": 30.00,
            "currency": "USD"
        }
    }'::jsonb
),
(
    'c0000000-0000-0000-0000-000000000002',
    'BETA',
    'Beta Wealth',
    '{
        "deposit_limit": 5000,
        "ineligible_account_types": ["IRA"],
        "contribution_cap": 7000,
        "fees": {
            "returned_check": 30.00,
            "currency": "USD"
        }
    }'::jsonb
)
ON CONFLICT (code) DO NOTHING;

-- Accounts
INSERT INTO accounts (id, code, correspondent_id, type) VALUES
-- Alpha investor accounts
('a0000000-0000-0000-0000-000000000001', 'ALPHA-001', 'c0000000-0000-0000-0000-000000000001', 'INDIVIDUAL'),
('a0000000-0000-0000-0000-000000000002', 'ALPHA-002', 'c0000000-0000-0000-0000-000000000001', 'INDIVIDUAL'),
('a0000000-0000-0000-0000-000000000003', 'ALPHA-003', 'c0000000-0000-0000-0000-000000000001', 'INDIVIDUAL'),
('a0000000-0000-0000-0000-000000000004', 'ALPHA-004', 'c0000000-0000-0000-0000-000000000001', 'INDIVIDUAL'),
('a0000000-0000-0000-0000-000000000005', 'ALPHA-005', 'c0000000-0000-0000-0000-000000000001', 'INDIVIDUAL'),
('a0000000-0000-0000-0000-000000000006', 'ALPHA-IRA', 'c0000000-0000-0000-0000-000000000001', 'IRA'),
-- Beta investor accounts
('a0000000-0000-0000-0000-000000000007', 'BETA-001',  'c0000000-0000-0000-0000-000000000002', 'INDIVIDUAL'),
('a0000000-0000-0000-0000-000000000008', 'BETA-002',  'c0000000-0000-0000-0000-000000000002', 'INDIVIDUAL'),
('a0000000-0000-0000-0000-000000000009', 'BETA-IRA',  'c0000000-0000-0000-0000-000000000002', 'IRA'),
-- Omnibus accounts
('a0000000-0000-0000-0000-000000000010', 'OMNIBUS-ALPHA', 'c0000000-0000-0000-0000-000000000001', 'OMNIBUS'),
('a0000000-0000-0000-0000-000000000011', 'OMNIBUS-BETA',  'c0000000-0000-0000-0000-000000000002', 'OMNIBUS'),
-- Fee account (no correspondent)
('a0000000-0000-0000-0000-000000000012', 'FEE-APEX', NULL, 'FEE')
ON CONFLICT (code) DO NOTHING;
